package application

import (
	"time"

	"wildframe/internal/domain"
	"wildframe/internal/evidence"
)

type Role string

const (
	RoleOrganizer Role = "organizer"
	RoleAnnotator Role = "annotator"
	RoleExpert    Role = "expert"
	RolePublisher Role = "publisher"
)

type CommandMeta struct {
	Actor           string `json:"actor"`
	Role            Role   `json:"role"`
	ExpectedVersion int64  `json:"expectedVersion"`
	IdempotencyKey  string `json:"idempotencyKey"`
}

type CreateCollectionCommand struct {
	CommandMeta
	ReserveName    string    `json:"reserveName"`
	CameraSite     string    `json:"cameraSite"`
	CapturedFrom   time.Time `json:"capturedFrom"`
	CapturedTo     time.Time `json:"capturedTo"`
	RuleSetVersion string    `json:"ruleSetVersion"`
	SeatA          string    `json:"seatA"`
	SeatB          string    `json:"seatB"`
}

type RegisterEvidenceCommand struct {
	CommandMeta
	OriginalName string    `json:"originalName"`
	CapturedAt   time.Time `json:"capturedAt"`
	CameraSite   string    `json:"cameraSite"`
	SHA256Digest string    `json:"sha256Digest"`
	MediaType    string    `json:"mediaType"`
	PixelWidth   int       `json:"pixelWidth"`
	PixelHeight  int       `json:"pixelHeight"`
	Payload      []byte    `json:"-"`
}

type BatchEvidenceItem struct {
	ClientItemID string    `json:"clientItemID"`
	OriginalName string    `json:"originalName"`
	CapturedAt   time.Time `json:"capturedAt"`
	CameraSite   string    `json:"cameraSite"`
	PixelWidth   int       `json:"pixelWidth"`
	PixelHeight  int       `json:"pixelHeight"`
	Payload      []byte    `json:"-"`
}

type RegisterEvidenceBatchCommand struct {
	CommandMeta
	Items []BatchEvidenceItem `json:"items"`
}

type BatchEvidenceItemResult struct {
	ClientItemID string                `json:"clientItemID"`
	OriginalName string                `json:"originalName"`
	Status       string                `json:"status"`
	Code         string                `json:"code,omitempty"`
	Message      string                `json:"message"`
	Evidence     *domain.ImageEvidence `json:"evidence,omitempty"`
}

type RegisterEvidenceBatchResult struct {
	CollectionID string                    `json:"collectionID"`
	Fingerprint  string                    `json:"fingerprint"`
	Version      int64                     `json:"version"`
	Items        []BatchEvidenceItemResult `json:"items"`
}

type SubmitAnnotationCommand struct {
	CommandMeta
	EvidenceID           string  `json:"evidenceID"`
	SeatID               string  `json:"seatID"`
	SpeciesCode          string  `json:"speciesCode"`
	IndividualCount      int     `json:"individualCount"`
	Confidence           float64 `json:"confidence"`
	Identifiability      string  `json:"identifiability"`
	UnidentifiableReason string  `json:"unidentifiableReason"`
	Note                 string  `json:"note"`
	SupersedesRevisionID string  `json:"supersedesRevisionID,omitempty"`
}

type RecalculateQualityCommand struct{ CommandMeta }

type AdjudicationPreview struct {
	Valid             bool     `json:"valid"`
	Message           string   `json:"message"`
	EvidenceID        string   `json:"evidenceID"`
	Action            string   `json:"action"`
	FinalSpeciesCode  string   `json:"finalSpeciesCode,omitempty"`
	FinalCount        int      `json:"finalCount,omitempty"`
	TargetSeatID      string   `json:"targetSeatID,omitempty"`
	ClosingFindingIDs []string `json:"closingFindingIDs,omitempty"`
}

type AdjudicateCommand struct {
	CommandMeta
	EvidenceID       string `json:"evidenceID"`
	Action           string `json:"action"`
	FinalSpeciesCode string `json:"finalSpeciesCode"`
	FinalCount       int    `json:"finalCount"`
	Rationale        string `json:"rationale"`
	TargetSeatID     string `json:"targetSeatID"`
}

type ReviewCommand struct{ CommandMeta }

type FreezeCommand struct {
	CommandMeta
	ManifestDigest string `json:"manifestDigest"`
}

type IssueCommand struct{ CommandMeta }

type CollectionView struct {
	Collection        domain.ImageCollection        `json:"collection"`
	Evidence          []EvidenceView                `json:"evidence"`
	Findings          []domain.QualityFinding       `json:"findings"`
	Decisions         []domain.AdjudicationDecision `json:"decisions"`
	Audit             []domain.AuditRecord          `json:"audit"`
	Manifest          any                           `json:"manifest,omitempty"`
	Credential        *evidence.CredentialEnvelope  `json:"credential,omitempty"`
	SeatProgress      *SeatProgress                 `json:"seatProgress,omitempty"`
	QualitySummary    QualitySummary                `json:"qualitySummary"`
	QualityRuns       []domain.QualityRun           `json:"qualityRuns"`
	RemediationTasks  []domain.RemediationTask      `json:"remediationTasks"`
	AdjudicationQueue []AdjudicationQueueItem       `json:"adjudicationQueue"`
	AuditIntegrity    AuditIntegrity                `json:"auditIntegrity"`
	StatusDurations   []StatusDuration              `json:"statusDurations,omitempty"`
}

type CollectionQuery struct {
	ViewerSeat      string
	TaskFilter      string
	Sort            string
	FindingRule     string
	FindingSeverity domain.Severity
	FindingStatus   domain.FindingStatus
	QueueFilter     string
}

type EvidenceView struct {
	domain.ImageEvidence
	Annotations    []domain.AnnotationRevision `json:"annotations"`
	SeatVisibility []SeatVisibility            `json:"seatVisibility"`
}

type SeatVisibility struct {
	SeatID    string                     `json:"seatID"`
	Submitted bool                       `json:"submitted"`
	Revealed  bool                       `json:"revealed"`
	Revision  *domain.AnnotationRevision `json:"revision,omitempty"`
}

type SeatProgress struct {
	SeatID         string `json:"seatID"`
	Total          int    `json:"total"`
	Submitted      int    `json:"submitted"`
	Pending        int    `json:"pending"`
	Remediation    int    `json:"remediation"`
	NextEvidenceID string `json:"nextEvidenceID,omitempty"`
}

type QualityCount struct {
	RuleCode string               `json:"ruleCode"`
	Severity domain.Severity      `json:"severity"`
	Status   domain.FindingStatus `json:"status"`
	Count    int                  `json:"count"`
}
type QualitySummary struct {
	OpenBlocking     int            `json:"openBlocking"`
	OpenWarnings     int            `json:"openWarnings"`
	AffectedEvidence int            `json:"affectedEvidence"`
	Groups           []QualityCount `json:"groups"`
}

type AdjudicationQueueItem struct {
	EvidenceID      string                       `json:"evidenceID"`
	BlockingCount   int                          `json:"blockingCount"`
	FirstOpenedAt   time.Time                    `json:"firstOpenedAt"`
	Status          string                       `json:"status"`
	Findings        []domain.QualityFinding      `json:"findings"`
	Revisions       []domain.AnnotationRevision  `json:"revisions"`
	Decision        *domain.AdjudicationDecision `json:"decision,omitempty"`
	RemediationTask *domain.RemediationTask      `json:"remediationTask,omitempty"`
}

type ManifestPreview struct {
	Manifest          evidence.Manifest             `json:"manifest"`
	Digest            string                        `json:"digest"`
	CollectionVersion int64                         `json:"collectionVersion"`
	GeneratedAt       time.Time                     `json:"generatedAt"`
	Expired           bool                          `json:"expired"`
	Statistics        evidence.ManifestStatistics   `json:"statistics"`
	Diff              []evidence.ManifestDifference `json:"diff"`
}

type VerificationResult struct {
	Valid           bool      `json:"valid"`
	Message         string    `json:"message"`
	SignatureValid  bool      `json:"signatureValid"`
	SignatureStatus string    `json:"signatureStatus"`
	BindingStatus   string    `json:"bindingStatus"`
	CredentialID    string    `json:"credentialID"`
	CollectionID    string    `json:"collectionID"`
	ManifestDigest  string    `json:"manifestDigest"`
	ManifestVersion int       `json:"manifestVersion"`
	IssuedBy        string    `json:"issuedBy"`
	IssuedAt        time.Time `json:"issuedAt"`
}

type AuditIntegrity struct {
	Valid   bool   `json:"valid"`
	Message string `json:"message"`
}
type StatusDuration struct {
	Status          domain.CollectionStatus `json:"status"`
	FirstEnteredAt  time.Time               `json:"firstEnteredAt"`
	LastLeftAt      *time.Time              `json:"lastLeftAt,omitempty"`
	DurationSeconds int64                   `json:"durationSeconds"`
	Current         bool                    `json:"current"`
}
type AuditQuery struct {
	Actor, Action        string
	From, To             *time.Time
	FromStatus, ToStatus domain.CollectionStatus
	After                uint64
	Limit                int
}
type AuditPage struct {
	Records         []domain.AuditRecord `json:"records"`
	NextCursor      uint64               `json:"nextCursor,omitempty"`
	Integrity       AuditIntegrity       `json:"integrity"`
	StatusDurations []StatusDuration     `json:"statusDurations,omitempty"`
}
