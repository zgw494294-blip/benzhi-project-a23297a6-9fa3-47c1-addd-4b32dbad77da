package persistence

import (
	"encoding/json"

	"wildframe/internal/domain"
)

type IdempotencyRecord struct {
	Fingerprint string          `json:"fingerprint"`
	Result      json.RawMessage `json:"result"`
}

type State struct {
	Collections      map[string]domain.ImageCollection                 `json:"collections"`
	Evidence         map[string][]domain.ImageEvidence                 `json:"evidence"`
	Revisions        map[string][]domain.AnnotationRevision            `json:"revisions"`
	Findings         map[string][]domain.QualityFinding                `json:"findings"`
	Decisions        map[string]map[string]domain.AdjudicationDecision `json:"decisions"`
	DecisionHistory  map[string][]domain.AdjudicationDecision          `json:"decisionHistory"`
	RemediationTasks map[string][]domain.RemediationTask               `json:"remediationTasks"`
	QualityRuns      map[string][]domain.QualityRun                    `json:"qualityRuns"`
	ManifestPreviews map[string]domain.ManifestPreviewRecord           `json:"manifestPreviews"`
	Credentials      map[string]json.RawMessage                        `json:"credentials"`
	Manifests        map[string]json.RawMessage                        `json:"manifests"`
	Audits           map[string][]domain.AuditRecord                   `json:"audits"`
	Idempotency      map[string]IdempotencyRecord                      `json:"idempotency"`
}

func NewState() State {
	return State{
		Collections: map[string]domain.ImageCollection{}, Evidence: map[string][]domain.ImageEvidence{},
		Revisions: map[string][]domain.AnnotationRevision{}, Findings: map[string][]domain.QualityFinding{},
		Decisions: map[string]map[string]domain.AdjudicationDecision{}, DecisionHistory: map[string][]domain.AdjudicationDecision{},
		RemediationTasks: map[string][]domain.RemediationTask{}, QualityRuns: map[string][]domain.QualityRun{},
		ManifestPreviews: map[string]domain.ManifestPreviewRecord{}, Credentials: map[string]json.RawMessage{},
		Manifests: map[string]json.RawMessage{}, Audits: map[string][]domain.AuditRecord{},
		Idempotency: map[string]IdempotencyRecord{},
	}
}

// cloneState produces a deep copy of the projection state. A deep copy is
// required so that mutations made by an Update callback to slice elements,
// nested maps, or embedded byte buffers cannot leak back into the committed
// in-memory projection when a later step (ledger append or snapshot write)
// fails and the transaction must be discarded.
func cloneState(state State) (State, error) {
	return State{
		Collections:      cloneMap(state.Collections),
		Evidence:         cloneSliceMap(state.Evidence),
		Revisions:        cloneSliceMap(state.Revisions),
		Findings:         cloneSliceMap(state.Findings),
		Decisions:        cloneNestedMap(state.Decisions),
		DecisionHistory:  cloneSliceMap(state.DecisionHistory),
		RemediationTasks: cloneSliceMap(state.RemediationTasks),
		QualityRuns:      cloneSliceMap(state.QualityRuns),
		ManifestPreviews: cloneManifestPreviews(state.ManifestPreviews),
		Credentials:      cloneRawMessageMap(state.Credentials),
		Manifests:        cloneRawMessageMap(state.Manifests),
		Audits:           cloneSliceMap(state.Audits),
		Idempotency:      cloneIdempotency(state.Idempotency),
	}, nil
}

func cloneMap[K comparable, V any](source map[K]V) map[K]V {
	clone := make(map[K]V, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

// cloneSliceMap deep-copies maps whose values are slices. The slice backing
// arrays are duplicated so that in-place element mutations on the clone cannot
// leak back into the committed projection when a ledger append fails after the
// mutation callback has run.
func cloneSliceMap[K comparable, V any](source map[K][]V) map[K][]V {
	clone := make(map[K][]V, len(source))
	for key, value := range source {
		duplicate := make([]V, len(value))
		copy(duplicate, value)
		clone[key] = duplicate
	}
	return clone
}

// cloneNestedMap deep-copies maps whose values are themselves maps, preventing
// writes to inner maps in the clone from leaking back into the committed
// projection.
func cloneNestedMap[K comparable, K2 comparable, V any](source map[K]map[K2]V) map[K]map[K2]V {
	clone := make(map[K]map[K2]V, len(source))
	for key, inner := range source {
		duplicate := make(map[K2]V, len(inner))
		for innerKey, innerValue := range inner {
			duplicate[innerKey] = innerValue
		}
		clone[key] = duplicate
	}
	return clone
}

// cloneRawMessageMap deep-copies maps whose values are json.RawMessage byte
// slices, so replacing or appending bytes on a cloned value cannot alias the
// committed byte buffer.
func cloneRawMessageMap[K comparable](source map[K]json.RawMessage) map[K]json.RawMessage {
	clone := make(map[K]json.RawMessage, len(source))
	for key, value := range source {
		clone[key] = append(json.RawMessage(nil), value...)
	}
	return clone
}

// cloneManifestPreviews deep-copies manifest preview records, duplicating the
// embedded Canonical byte slice so that byte-level mutations on the clone are
// isolated from the committed projection.
func cloneManifestPreviews(source map[string]domain.ManifestPreviewRecord) map[string]domain.ManifestPreviewRecord {
	clone := make(map[string]domain.ManifestPreviewRecord, len(source))
	for key, value := range source {
		value.Canonical = append([]byte(nil), value.Canonical...)
		clone[key] = value
	}
	return clone
}

// cloneIdempotency deep-copies idempotency records, duplicating the embedded
// Result byte slice so that byte-level mutations on the clone are isolated
// from the committed projection.
func cloneIdempotency(source map[string]IdempotencyRecord) map[string]IdempotencyRecord {
	clone := make(map[string]IdempotencyRecord, len(source))
	for key, value := range source {
		value.Result = append(json.RawMessage(nil), value.Result...)
		clone[key] = value
	}
	return clone
}
