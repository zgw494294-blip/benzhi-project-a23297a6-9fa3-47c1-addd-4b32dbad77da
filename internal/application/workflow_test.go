package application

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"wildframe/internal/domain"
	"wildframe/internal/evidence"
	"wildframe/internal/persistence"
)

func testService(t *testing.T) *Service {
	t.Helper()
	directory := t.TempDir()
	repo, err := persistence.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := persistence.NewBlobStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := evidence.NewSigner(nil)
	if err != nil {
		t.Fatal(err)
	}
	return NewService(repo, blobs, evidence.NewQualityEngine(), signer)
}

func TestConsensusWorkflowBlindnessAndIdempotency(t *testing.T) {
	service := testService(t)
	from := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	collection, err := service.CreateCollection(CreateCollectionCommand{CommandMeta: CommandMeta{Actor: "o", Role: RoleOrganizer, IdempotencyKey: "create"}, ReserveName: "保护区", CameraSite: "cam", CapturedFrom: from, CapturedTo: from.Add(time.Hour), RuleSetVersion: "v1", SeatA: "a", SeatB: "b"})
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01, 0xff, 0xd9}
	sum := sha256.Sum256(payload)
	item, err := service.RegisterEvidence(collection.CollectionID, RegisterEvidenceCommand{CommandMeta: CommandMeta{Actor: "o", Role: RoleOrganizer, ExpectedVersion: collection.Version, IdempotencyKey: "upload"}, OriginalName: "a.jpg", CapturedAt: from.Add(time.Minute), CameraSite: "cam", SHA256Digest: hex.EncodeToString(sum[:]), MediaType: "image/jpeg", PixelWidth: 20, PixelHeight: 20, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	collection.Version++
	commandA := SubmitAnnotationCommand{CommandMeta: CommandMeta{Actor: "a", Role: RoleAnnotator, ExpectedVersion: collection.Version, IdempotencyKey: "a1"}, EvidenceID: item.EvidenceID, SeatID: "a", SpeciesCode: "ursus", IndividualCount: 1, Confidence: .9, Identifiability: "identifiable"}
	revisionA, err := service.SubmitAnnotation(collection.CollectionID, commandA)
	if err != nil {
		t.Fatal(err)
	}
	viewB, err := service.GetCollection(collection.CollectionID, "b")
	if err != nil {
		t.Fatal(err)
	}
	if len(viewB.Evidence[0].Annotations) != 0 {
		t.Fatal("席位 B 在提交前看到了席位 A 判断")
	}
	reused, err := service.SubmitAnnotation(collection.CollectionID, commandA)
	if err != nil || reused.RevisionID != revisionA.RevisionID {
		t.Fatalf("幂等复用失败: %v", err)
	}
	collection.Version++
	_, err = service.SubmitAnnotation(collection.CollectionID, SubmitAnnotationCommand{CommandMeta: CommandMeta{Actor: "b", Role: RoleAnnotator, ExpectedVersion: collection.Version, IdempotencyKey: "b1"}, EvidenceID: item.EvidenceID, SeatID: "b", SpeciesCode: "ursus", IndividualCount: 1, Confidence: .8, Identifiability: "identifiable"})
	if err != nil {
		t.Fatal(err)
	}
	collection.Version++
	collection, err = service.ApproveReview(collection.CollectionID, ReviewCommand{CommandMeta: CommandMeta{Actor: "expert", Role: RoleExpert, ExpectedVersion: collection.Version, IdempotencyKey: "review"}})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := service.PreviewManifest(collection.CollectionID)
	if err != nil {
		t.Fatal(err)
	}
	collection, err = service.Freeze(collection.CollectionID, FreezeCommand{CommandMeta: CommandMeta{Actor: "publisher", Role: RolePublisher, ExpectedVersion: collection.Version, IdempotencyKey: "freeze"}, ManifestDigest: preview.Digest})
	if err != nil {
		t.Fatal(err)
	}
	credential, err := service.IssueCredential(collection.CollectionID, IssueCommand{CommandMeta: CommandMeta{Actor: "publisher", Role: RolePublisher, ExpectedVersion: collection.Version, IdempotencyKey: "issue"}})
	if err != nil {
		t.Fatal(err)
	}
	if result := service.VerifyCredential(credential); !result.Valid {
		t.Fatal(result.Message)
	}
	final, err := service.GetCollection(collection.CollectionID, "")
	if err != nil || final.Collection.Status != domain.StatusReleased {
		t.Fatalf("终态错误: %v %s", err, final.Collection.Status)
	}
}

func TestStaleWriteRejected(t *testing.T) {
	service := testService(t)
	from := time.Now().UTC()
	collection, err := service.CreateCollection(CreateCollectionCommand{CommandMeta: CommandMeta{Actor: "o", Role: RoleOrganizer, IdempotencyKey: "create"}, ReserveName: "保护区", CameraSite: "cam", CapturedFrom: from, CapturedTo: from.Add(time.Hour), RuleSetVersion: "v1", SeatA: "a", SeatB: "b"})
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte{0xff, 0xd8, 0xff, 0xd9}
	sum := sha256.Sum256(payload)
	_, err = service.RegisterEvidence(collection.CollectionID, RegisterEvidenceCommand{CommandMeta: CommandMeta{Actor: "o", Role: RoleOrganizer, ExpectedVersion: collection.Version - 1, IdempotencyKey: "stale"}, OriginalName: "x.jpg", CapturedAt: from, CameraSite: "cam", SHA256Digest: hex.EncodeToString(sum[:]), MediaType: "image/jpeg", PixelWidth: 1, PixelHeight: 1, Payload: payload})
	if domain.ErrorCodeOf(err) != domain.CodeVersionConflict {
		t.Fatalf("期望版本冲突，实际 %v", err)
	}
}
