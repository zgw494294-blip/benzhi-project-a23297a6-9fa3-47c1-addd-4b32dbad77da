package persistence

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

type Repository struct {
	mu           sync.RWMutex
	directory    string
	ledgerPath   string
	snapshotPath string
	state        State
	sequence     uint64
	head         string
	recovered    bool
}

type RecoveryStatus struct {
	Recovered  bool   `json:"recovered"`
	Sequence   uint64 `json:"sequence"`
	LedgerHead string `json:"ledgerHead"`
}

func Open(directory string) (*Repository, error) {
	if directory == "" {
		return nil, fmt.Errorf("数据目录不能为空")
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, err
	}
	// Hold a shared lock while recovering so that a concurrent writer cannot
	// append to the ledger (or rewrite the snapshot) while we are reading it.
	lock, err := newFileLock(directory, syscall.LOCK_SH)
	if err != nil {
		return nil, err
	}
	defer lock.release()
	repo := &Repository{directory: directory, ledgerPath: filepath.Join(directory, "events.jsonl"), snapshotPath: filepath.Join(directory, "projection.json"), state: NewState()}
	events, err := readLedger(repo.ledgerPath)
	if err != nil {
		return nil, err
	}
	snapshot, snapshotState, err := readSnapshot(repo.snapshotPath)
	if err != nil {
		return nil, err
	}
	if len(events) > 0 {
		last := events[len(events)-1]
		if err := json.Unmarshal(last.Projection, &repo.state); err != nil {
			return nil, fmt.Errorf("重放最终投影失败: %w", err)
		}
		repo.sequence, repo.head = last.Sequence, last.Hash
		if snapshot.Sequence > repo.sequence || (snapshot.Sequence == repo.sequence && snapshot.LedgerHead != repo.head) {
			return nil, fmt.Errorf("投影快照与事件账本不一致")
		}
	} else if snapshot.Sequence != 0 {
		return nil, fmt.Errorf("存在无事件账本支持的投影快照")
	} else {
		repo.state = snapshotState
	}
	repo.recovered = true
	return repo, nil
}

func (r *Repository) Recovered() bool { r.mu.RLock(); defer r.mu.RUnlock(); return r.recovered }

func (r *Repository) RecoveryStatus() RecoveryStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return RecoveryStatus{Recovered: r.recovered, Sequence: r.sequence, LedgerHead: r.head}
}

func (r *Repository) Read(fn func(State) error) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	copy, err := cloneState(r.state)
	if err != nil {
		return err
	}
	return fn(copy)
}

func (r *Repository) Update(eventType, collectionID string, now time.Time, fn func(*State) error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Acquire an exclusive cross-process lock so that concurrent service
	// instances sharing the same data directory serialize their appends. The
	// in-memory mutex above only guards this process; the file lock guards the
	// whole ledger/snapshot pair across processes.
	lock, err := newFileLock(r.directory, syscall.LOCK_EX)
	if err != nil {
		return err
	}
	defer lock.release()
	// Re-derive the authoritative sequence, head and current projection from
	// the durable ledger. Another instance may have appended events after this
	// instance opened the directory, so the in-memory values can be stale and
	// must not be trusted to compute the next event. Building the next event on
	// top of the on-disk tail keeps the sequence/hash chain intact and ensures
	// the emitted projection never silently drops another instance's changes.
	sequence, head, projection, err := ledgerTail(r.ledgerPath)
	if err != nil {
		return err
	}
	r.sequence, r.head = sequence, head
	current := r.state
	if len(projection) > 0 {
		current = NewState()
		if err := json.Unmarshal(projection, &current); err != nil {
			return fmt.Errorf("重放最终投影失败: %w", err)
		}
		r.state = current
	}
	next, err := cloneState(current)
	if err != nil {
		return err
	}
	if err := fn(&next); err != nil {
		return err
	}
	projectionBytes, err := json.Marshal(next)
	if err != nil {
		return err
	}
	event := LedgerEvent{SchemaVersion: 1, Sequence: r.sequence + 1, PreviousHash: r.head, EventType: eventType, CollectionID: collectionID, OccurredAt: now.UTC().Format(time.RFC3339Nano), Projection: projectionBytes}
	if err := appendLedger(r.ledgerPath, event); err != nil {
		return err
	}
	event.Hash, _ = eventHash(event)
	r.state, r.sequence, r.head = next, event.Sequence, event.Hash
	if err := writeSnapshot(r.snapshotPath, event.Sequence, event.Hash, next); err != nil {
		return err
	}
	return nil
}

func (r *Repository) Directory() string { return r.directory }
