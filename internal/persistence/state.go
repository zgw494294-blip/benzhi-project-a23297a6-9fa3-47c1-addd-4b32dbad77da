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

type recoveryState struct {
	Collections     map[string]domain.ImageCollection                 `json:"collections"`
	Evidence        map[string][]domain.ImageEvidence                 `json:"evidence"`
	Revisions       map[string][]domain.AnnotationRevision            `json:"revisions"`
	Findings        map[string][]domain.QualityFinding                `json:"findings"`
	Decisions       map[string]map[string]domain.AdjudicationDecision `json:"decisions"`
	DecisionHistory map[string][]domain.AdjudicationDecision          `json:"decisionHistory"`
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

func cloneState(state State) (State, error) {
	raw, err := json.Marshal(state)
	if err != nil {
		return State{}, err
	}
	copy := NewState()
	if err := json.Unmarshal(raw, &copy); err != nil {
		return State{}, err
	}
	return copy, nil
}

func decodeRecoveryState(raw []byte) (State, error) {
	var recovered recoveryState
	if err := json.Unmarshal(raw, &recovered); err != nil {
		return State{}, err
	}
	state := NewState()
	state.Collections = recovered.Collections
	state.Evidence = recovered.Evidence
	state.Revisions = recovered.Revisions
	state.Findings = recovered.Findings
	state.Decisions = recovered.Decisions
	state.DecisionHistory = recovered.DecisionHistory
	return state, nil
}
