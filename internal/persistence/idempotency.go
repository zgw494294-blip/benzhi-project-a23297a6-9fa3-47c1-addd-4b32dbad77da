package persistence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"wildframe/internal/domain"
)

func Fingerprint(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func Reuse(state State, scope, key, fingerprint string, target any) (bool, error) {
	record, ok := state.Idempotency[scope+":"+key]
	if !ok {
		return false, nil
	}
	if record.Fingerprint != fingerprint {
		return false, domain.NewError(domain.CodeIdempotencyConflict, "同一 idempotencyKey 对应了不同请求")
	}
	if err := json.Unmarshal(record.Result, target); err != nil {
		return false, err
	}
	return true, nil
}

func Remember(state *State, scope, key, fingerprint string, result any) error {
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	state.Idempotency[scope+":"+key] = IdempotencyRecord{Fingerprint: fingerprint, Result: raw}
	return nil
}
