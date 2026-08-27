package evidence

import (
	"testing"
	"time"

	"wildframe/internal/domain"
)

func TestQualityDisagreementAndCredential(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := domain.ImageCollection{CollectionID: "c", CameraSite: "s", CapturedFrom: now, CapturedTo: now.Add(time.Hour), AnnotatorSeats: [2]string{"a", "b"}}
	e := domain.ImageEvidence{EvidenceID: "e", CollectionID: "c", CameraSite: "s", CapturedAt: now}
	seats := map[string]domain.AnnotationRevision{
		"a": {RevisionID: "r1", SeatID: "a", SpeciesCode: "ursus", IndividualCount: 1, Confidence: .9, Identifiability: "identifiable"},
		"b": {RevisionID: "r2", SeatID: "b", SpeciesCode: "vulpes", IndividualCount: 1, Confidence: .9, Identifiability: "identifiable"},
	}
	engine := NewQualityEngine()
	engine.Now = func() time.Time { return now }
	if got := BlockingCount(engine.EvaluateEvidence(c, e, seats)); got != 1 {
		t.Fatalf("blocking=%d", got)
	}
	signer, err := NewSigner(nil)
	if err != nil {
		t.Fatal(err)
	}
	credential := signer.Issue("x", "c", "digest", "owner", 1, now)
	if valid, reason := VerifyCredential(credential, "digest"); !valid {
		t.Fatal(reason)
	}
}
