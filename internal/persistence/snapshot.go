package persistence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type snapshotFile struct {
	SchemaVersion int             `json:"schemaVersion"`
	Sequence      uint64          `json:"sequence"`
	LedgerHead    string          `json:"ledgerHead"`
	Projection    json.RawMessage `json:"projection"`
	Checksum      string          `json:"checksum"`
}

func writeSnapshot(path string, sequence uint64, head string, state State) error {
	projection, err := json.Marshal(state)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(projection)
	snapshot := snapshotFile{SchemaVersion: 1, Sequence: sequence, LedgerHead: head, Projection: projection, Checksum: hex.EncodeToString(sum[:])}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".projection-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func readSnapshot(path string) (snapshotFile, State, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return snapshotFile{}, NewState(), nil
	}
	if err != nil {
		return snapshotFile{}, State{}, err
	}
	var snapshot snapshotFile
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return snapshotFile{}, State{}, err
	}
	sum := sha256.Sum256(snapshot.Projection)
	if snapshot.SchemaVersion != 1 || snapshot.Checksum != hex.EncodeToString(sum[:]) {
		return snapshotFile{}, State{}, fmt.Errorf("投影快照校验失败")
	}
	state := NewState()
	if err := json.Unmarshal(snapshot.Projection, &state); err != nil {
		return snapshotFile{}, State{}, err
	}
	return snapshot, state, nil
}
