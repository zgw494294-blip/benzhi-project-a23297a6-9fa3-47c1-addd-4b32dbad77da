package domain

type CollectionStatus string

const (
	StatusDraft       CollectionStatus = "draft"
	StatusAnnotating  CollectionStatus = "annotating"
	StatusArbitration CollectionStatus = "awaiting_adjudication"
	StatusReview      CollectionStatus = "awaiting_review"
	StatusRemediation CollectionStatus = "remediating"
	StatusFreezable   CollectionStatus = "freezable"
	StatusFrozen      CollectionStatus = "frozen"
	StatusReleased    CollectionStatus = "released"
)

func (s CollectionStatus) Mutable() bool {
	return s != StatusFrozen && s != StatusReleased
}

func (s CollectionStatus) CanAnnotate() bool {
	return s == StatusAnnotating || s == StatusRemediation
}

func ValidStatusTransition(from, to CollectionStatus) bool {
	if from == to {
		return true
	}
	return validTransition(from, to)
}

type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityBlocking Severity = "blocking"
)

type FindingStatus string

const (
	FindingOpen     FindingStatus = "open"
	FindingResolved FindingStatus = "resolved"
)
