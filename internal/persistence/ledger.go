package persistence

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
)

type LedgerEvent struct {
	SchemaVersion int             `json:"schemaVersion"`
	Sequence      uint64          `json:"sequence"`
	PreviousHash  string          `json:"previousHash"`
	EventType     string          `json:"eventType"`
	CollectionID  string          `json:"collectionID"`
	OccurredAt    string          `json:"occurredAt"`
	Projection    json.RawMessage `json:"projection"`
	Hash          string          `json:"hash"`
}

func eventHash(event LedgerEvent) (string, error) {
	event.Hash = ""
	raw, err := json.Marshal(event)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func appendLedger(path string, event LedgerEvent) error {
	hash, err := eventHash(event)
	if err != nil {
		return err
	}
	event.Hash = hash
	raw, err := json.Marshal(event)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write(append(raw, '\n')); err != nil {
		return err
	}
	return file.Sync()
}

func readLedger(path string) ([]LedgerEvent, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	events, err := scanLedger(file)
	if err != nil {
		return nil, err
	}
	return events, nil
}

// ledgerTail returns the authoritative sequence, head and final projection
// recorded on disk. It is used by concurrent writers to derive the next event
// from the durable ledger rather than from stale in-memory state, so that
// multiple process instances appending to the same file never break the chain
// and never emit a projection that discards another instance's changes. The
// chain is validated while scanning, mirroring readLedger.
func ledgerTail(path string) (sequence uint64, head string, projection json.RawMessage, err error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return 0, "", nil, nil
	}
	if err != nil {
		return 0, "", nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 32*1024*1024)
	previous := ""
	for scanner.Scan() {
		var event LedgerEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return 0, "", nil, fmt.Errorf("事件账本 JSON 损坏: %w", err)
		}
		if event.SchemaVersion != 1 || event.Sequence != sequence+1 || event.PreviousHash != previous {
			return 0, "", nil, fmt.Errorf("事件账本序号或前序摘要链无效")
		}
		hash, err := eventHash(event)
		if err != nil || hash != event.Hash {
			return 0, "", nil, fmt.Errorf("事件账本摘要校验失败")
		}
		sequence, previous, head, projection = event.Sequence, event.Hash, event.Hash, event.Projection
	}
	if err := scanner.Err(); err != nil {
		return 0, "", nil, err
	}
	return sequence, head, projection, nil
}

func scanLedger(file *os.File) ([]LedgerEvent, error) {
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 32*1024*1024)
	events := make([]LedgerEvent, 0)
	previous := ""
	var sequence uint64
	for scanner.Scan() {
		var event LedgerEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, fmt.Errorf("事件账本 JSON 损坏: %w", err)
		}
		if event.SchemaVersion != 1 || event.Sequence != sequence+1 || event.PreviousHash != previous {
			return nil, fmt.Errorf("事件账本序号或前序摘要链无效")
		}
		hash, err := eventHash(event)
		if err != nil || hash != event.Hash {
			return nil, fmt.Errorf("事件账本摘要校验失败")
		}
		events = append(events, event)
		sequence, previous = event.Sequence, event.Hash
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return events, nil
}
