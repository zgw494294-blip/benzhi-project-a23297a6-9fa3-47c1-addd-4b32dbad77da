package blind_view_cache_crosses_seats_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"wildframe/internal/application"
	"wildframe/internal/evidence"
	"wildframe/internal/persistence"
	"wildframe/internal/web"
)

func TestBlindViewCacheKeepsSeatIsolationAndFreshness(t *testing.T) {
	directory := t.TempDir()
	repository, err := persistence.Open(directory)
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
	service := application.NewService(repository, blobs, evidence.NewQualityEngine(), signer)
	now := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	service.SetClock(func() time.Time { return now })

	collection, err := service.CreateCollection(application.CreateCollectionCommand{
		CommandMeta: application.CommandMeta{Actor: "organizer", Role: application.RoleOrganizer, IdempotencyKey: "create-cache-case"},
		ReserveName: "云岭保护区", CameraSite: "CAM-CACHE-1", CapturedFrom: now.Add(-time.Hour), CapturedTo: now.Add(time.Hour),
		RuleSetVersion: "rules-v1", SeatA: "seat-a", SeatB: "seat-b",
	})
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01, 0xff, 0xd9}
	digest := sha256.Sum256(payload)
	item, err := service.RegisterEvidence(collection.CollectionID, application.RegisterEvidenceCommand{
		CommandMeta:  application.CommandMeta{Actor: "organizer", Role: application.RoleOrganizer, ExpectedVersion: collection.Version, IdempotencyKey: "upload-cache-case"},
		OriginalName: "cache-case.jpg", CapturedAt: now, CameraSite: collection.CameraSite,
		SHA256Digest: hex.EncodeToString(digest[:]), MediaType: "image/jpeg", PixelWidth: 32, PixelHeight: 24, Payload: payload,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.SubmitAnnotation(collection.CollectionID, application.SubmitAnnotationCommand{
		CommandMeta: application.CommandMeta{Actor: "seat-a", Role: application.RoleAnnotator, ExpectedVersion: collection.Version + 1, IdempotencyKey: "annotate-a-cache-case"},
		EvidenceID:  item.EvidenceID, SeatID: "seat-a", SpeciesCode: "ursus-thibetanus", IndividualCount: 1, Confidence: 0.91, Identifiability: "identifiable",
	})
	if err != nil {
		t.Fatal(err)
	}

	handler := web.NewServer(service).Handler()
	viewA := getCollection(t, handler, collection.CollectionID, "seat-a")
	seatAForA, ok := visibility(viewA, "seat-a")
	if !ok || !seatAForA.Revealed || seatAForA.Revision == nil {
		t.Fatal("测试前提失败：席位 A 无法读取自己的已提交判断")
	}

	viewBBefore := getCollection(t, handler, collection.CollectionID, "seat-b")
	seatAForBBefore, _ := visibility(viewBBefore, "seat-a")
	leakedBeforeSubmission := seatAForBBefore.Revealed || seatAForBBefore.Revision != nil
	wrongViewerProgress := viewBBefore.SeatProgress == nil || viewBBefore.SeatProgress.SeatID != "seat-b"

	commandB := application.SubmitAnnotationCommand{
		CommandMeta: application.CommandMeta{Actor: "seat-b", Role: application.RoleAnnotator, ExpectedVersion: viewA.Collection.Version, IdempotencyKey: "annotate-b-cache-case"},
		EvidenceID:  item.EvidenceID, SeatID: "seat-b", SpeciesCode: "ursus-thibetanus", IndividualCount: 1, Confidence: 0.93, Identifiability: "identifiable",
	}
	body, err := json.Marshal(commandB)
	if err != nil {
		t.Fatal(err)
	}
	post := httptest.NewRequest(http.MethodPost, "/api/v1/collections/"+collection.CollectionID+"/annotations", bytes.NewReader(body))
	post.Header.Set("Content-Type", "application/json")
	postResult := httptest.NewRecorder()
	handler.ServeHTTP(postResult, post)
	if postResult.Code != http.StatusCreated {
		t.Fatalf("席位 B 提交失败: status=%d body=%s", postResult.Code, postResult.Body.String())
	}

	viewBAfter := getCollection(t, handler, collection.CollectionID, "seat-b")
	seatBAfter, ok := visibility(viewBAfter, "seat-b")
	missingOwnSubmission := !ok || !seatBAfter.Submitted || !seatBAfter.Revealed || seatBAfter.Revision == nil
	staleVersion := viewBAfter.Collection.Version != viewA.Collection.Version+1
	if leakedBeforeSubmission || wrongViewerProgress || missingOwnSubmission || staleVersion {
		t.Fatalf("盲标视图缓存跨席位复用且写后未失效: leakedBeforeSubmission=%t wrongViewerProgress=%t missingOwnSubmission=%t staleVersion=%t",
			leakedBeforeSubmission, wrongViewerProgress, missingOwnSubmission, staleVersion)
	}
}

func getCollection(t *testing.T, handler http.Handler, collectionID, seat string) application.CollectionView {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/collections/"+collectionID+"?seat="+seat, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("读取批次失败: status=%d body=%s", response.Code, response.Body.String())
	}
	var view application.CollectionView
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	return view
}

func visibility(view application.CollectionView, seat string) (application.SeatVisibility, bool) {
	if len(view.Evidence) == 0 {
		return application.SeatVisibility{}, false
	}
	for _, item := range view.Evidence[0].SeatVisibility {
		if item.SeatID == seat {
			return item, true
		}
	}
	return application.SeatVisibility{}, false
}
