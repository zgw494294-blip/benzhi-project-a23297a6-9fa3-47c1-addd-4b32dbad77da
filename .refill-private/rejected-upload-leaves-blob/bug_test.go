package rejecteduploadleavesblob

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"wildframe/internal/application"
	"wildframe/internal/domain"
	"wildframe/internal/evidence"
	"wildframe/internal/persistence"
)

func TestRejectedRegistrationDoesNotLeaveBlob(t *testing.T) {
	dir := t.TempDir()
	repo, err := persistence.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := persistence.NewBlobStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := evidence.NewSigner(nil)
	if err != nil {
		t.Fatal(err)
	}
	service := application.NewService(repo, blobs, evidence.NewQualityEngine(), signer)
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	collection, err := service.CreateCollection(application.CreateCollectionCommand{
		CommandMeta: application.CommandMeta{Actor: "organizer", Role: application.RoleOrganizer, IdempotencyKey: "create"},
		ReserveName: "保护区", CameraSite: "CAM-1", CapturedFrom: from, CapturedTo: from.Add(time.Hour), RuleSetVersion: "v1", SeatA: "a", SeatB: "b",
	})
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte{0xff, 0xd8, 0xff, 0xd9}
	sum := sha256.Sum256(payload)
	digest := hex.EncodeToString(sum[:])
	_, err = service.RegisterEvidence(collection.CollectionID, application.RegisterEvidenceCommand{
		CommandMeta:  application.CommandMeta{Actor: "organizer", Role: application.RoleOrganizer, ExpectedVersion: collection.Version - 1, IdempotencyKey: "stale"},
		OriginalName: "image.jpg", CapturedAt: from.Add(time.Minute), CameraSite: "CAM-1", SHA256Digest: digest,
		MediaType: "image/jpeg", PixelWidth: 10, PixelHeight: 10, Payload: payload,
	})
	if domain.ErrorCodeOf(err) != domain.CodeVersionConflict {
		t.Fatalf("未得到预期的版本冲突: %v", err)
	}
	file, openErr := blobs.Open(digest)
	if openErr == nil {
		file.Close()
		t.Fatal("版本冲突拒绝登记后仍遗留了不可达载荷")
	}
	if !os.IsNotExist(openErr) {
		t.Fatalf("检查载荷失败: %v", openErr)
	}
}
