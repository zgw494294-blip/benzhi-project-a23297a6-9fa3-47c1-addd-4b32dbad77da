package domain

import "time"

type ImageCollection struct {
	CollectionID    string           `json:"collectionID"`
	ReserveName     string           `json:"reserveName"`
	CameraSite      string           `json:"cameraSite"`
	CapturedFrom    time.Time        `json:"capturedFrom"`
	CapturedTo      time.Time        `json:"capturedTo"`
	RuleSetVersion  string           `json:"ruleSetVersion"`
	AnnotatorSeats  [2]string        `json:"annotatorSeats"`
	Status          CollectionStatus `json:"status"`
	Version         int64            `json:"version"`
	CreatedAt       time.Time        `json:"createdAt"`
	UpdatedAt       time.Time        `json:"updatedAt"`
	ReviewerID      string           `json:"reviewerID,omitempty"`
	ManifestDigest  string           `json:"manifestDigest,omitempty"`
	ManifestVersion int              `json:"manifestVersion,omitempty"`
}

type ImageEvidence struct {
	EvidenceID   string    `json:"evidenceID"`
	CollectionID string    `json:"collectionID"`
	OriginalName string    `json:"originalName"`
	CapturedAt   time.Time `json:"capturedAt"`
	CameraSite   string    `json:"cameraSite"`
	SHA256Digest string    `json:"sha256Digest"`
	MediaType    string    `json:"mediaType"`
	ByteSize     int64     `json:"byteSize"`
	PixelWidth   int       `json:"pixelWidth"`
	PixelHeight  int       `json:"pixelHeight"`
	BlobKey      string    `json:"blobKey"`
	RegisteredAt time.Time `json:"registeredAt"`
}

type AnnotationRevision struct {
	RevisionID           string    `json:"revisionID"`
	EvidenceID           string    `json:"evidenceID"`
	SeatID               string    `json:"seatID"`
	SpeciesCode          string    `json:"speciesCode"`
	IndividualCount      int       `json:"individualCount"`
	Confidence           float64   `json:"confidence"`
	Identifiability      string    `json:"identifiability"`
	UnidentifiableReason string    `json:"unidentifiableReason,omitempty"`
	Note                 string    `json:"note,omitempty"`
	SupersedesRevisionID string    `json:"supersedesRevisionID,omitempty"`
	SubmittedAt          time.Time `json:"submittedAt"`
}

type RemediationTask struct {
	TaskID             string     `json:"taskID"`
	CollectionID       string     `json:"collectionID"`
	EvidenceID         string     `json:"evidenceID"`
	Round              int        `json:"round"`
	TargetSeatID       string     `json:"targetSeatID"`
	Rationale          string     `json:"rationale"`
	BaselineRevisionID string     `json:"baselineRevisionID"`
	ResultRevisionID   string     `json:"resultRevisionID,omitempty"`
	Status             string     `json:"status"`
	CreatedAt          time.Time  `json:"createdAt"`
	ReturnedAt         *time.Time `json:"returnedAt,omitempty"`
}

type QualityFinding struct {
	FindingID         string        `json:"findingID"`
	CollectionID      string        `json:"collectionID"`
	EvidenceID        string        `json:"evidenceID"`
	RuleCode          string        `json:"ruleCode"`
	Severity          Severity      `json:"severity"`
	Status            FindingStatus `json:"status"`
	Details           string        `json:"details"`
	SourceRevisionIDs []string      `json:"sourceRevisionIDs"`
	OpenedAt          time.Time     `json:"openedAt"`
	ResolvedAt        *time.Time    `json:"resolvedAt,omitempty"`
}

type AdjudicationDecision struct {
	DecisionID       string    `json:"decisionID"`
	CollectionID     string    `json:"collectionID"`
	EvidenceID       string    `json:"evidenceID"`
	Action           string    `json:"action"`
	FinalSpeciesCode string    `json:"finalSpeciesCode,omitempty"`
	FinalCount       int       `json:"finalCount,omitempty"`
	Rationale        string    `json:"rationale"`
	TargetSeatID     string    `json:"targetSeatID,omitempty"`
	ReviewerID       string    `json:"reviewerID"`
	DecidedAt        time.Time `json:"decidedAt"`
}

type QualityRun struct {
	RunID               string    `json:"runID"`
	CollectionID        string    `json:"collectionID"`
	RuleSetVersion      string    `json:"ruleSetVersion"`
	InputRevisionDigest string    `json:"inputRevisionDigest"`
	ResultDigest        string    `json:"resultDigest"`
	FindingCount        int       `json:"findingCount"`
	Changed             bool      `json:"changed"`
	TriggeredBy         string    `json:"triggeredBy"`
	TriggeredAt         time.Time `json:"triggeredAt"`
}

type ManifestPreviewRecord struct {
	CollectionID      string    `json:"collectionID"`
	CollectionVersion int64     `json:"collectionVersion"`
	Digest            string    `json:"digest"`
	Canonical         []byte    `json:"canonical"`
	GeneratedAt       time.Time `json:"generatedAt"`
}

type ReleaseCredential struct {
	CredentialID       string    `json:"credentialID"`
	CollectionID       string    `json:"collectionID"`
	ManifestDigest     string    `json:"manifestDigest"`
	ManifestVersion    int       `json:"manifestVersion"`
	IssuedBy           string    `json:"issuedBy"`
	IssuedAt           time.Time `json:"issuedAt"`
	Algorithm          string    `json:"algorithm"`
	Signature          string    `json:"signature"`
	VerificationStatus string    `json:"verificationStatus"`
}

type AuditRecord struct {
	Sequence     uint64           `json:"sequence"`
	CollectionID string           `json:"collectionID"`
	Action       string           `json:"action"`
	Actor        string           `json:"actor"`
	FromStatus   CollectionStatus `json:"fromStatus,omitempty"`
	ToStatus     CollectionStatus `json:"toStatus,omitempty"`
	Details      string           `json:"details"`
	ObjectType   string           `json:"objectType,omitempty"`
	ObjectID     string           `json:"objectID,omitempty"`
	OccurredAt   time.Time        `json:"occurredAt"`
}
