package application

import (
	"testing"
	"time"

	"wildframe/internal/domain"
)

func createExtensionCollection(t *testing.T, service *Service) domain.ImageCollection {
	t.Helper()
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	collection, err := service.CreateCollection(CreateCollectionCommand{CommandMeta: CommandMeta{Actor: "organizer", Role: RoleOrganizer, IdempotencyKey: "create-extension"}, ReserveName: "测试保护区", CameraSite: "CAM-1", CapturedFrom: from, CapturedTo: from.Add(24 * time.Hour), RuleSetVersion: "v1", SeatA: "seat-a", SeatB: "seat-b"})
	if err != nil {
		t.Fatal(err)
	}
	return collection
}

func TestBatchRegistrationPartialDuplicateAndIdempotency(t *testing.T) {
	service := testService(t)
	collection := createExtensionCollection(t, service)
	captured := collection.CapturedFrom.Add(time.Hour)
	jpegA := []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01, 0xff, 0xd9}
	jpegB := append(append([]byte(nil), jpegA...), 0x00)
	command := RegisterEvidenceBatchCommand{CommandMeta: CommandMeta{Actor: "organizer", Role: RoleOrganizer, ExpectedVersion: collection.Version, IdempotencyKey: "batch-1"}, Items: []BatchEvidenceItem{
		{ClientItemID: "one", OriginalName: "one.jpg", CapturedAt: captured, CameraSite: "CAM-1", PixelWidth: 10, PixelHeight: 10, Payload: jpegA},
		{ClientItemID: "duplicate", OriginalName: "duplicate.jpg", CapturedAt: captured, CameraSite: "CAM-1", PixelWidth: 10, PixelHeight: 10, Payload: jpegA},
		{ClientItemID: "two", OriginalName: "two.jpg", CapturedAt: captured, CameraSite: "CAM-1", PixelWidth: 10, PixelHeight: 10, Payload: jpegB},
	}}
	result, err := service.RegisterEvidenceBatch(collection.CollectionID, command)
	if err != nil {
		t.Fatal(err)
	}
	if result.Items[0].Status != "registered" || result.Items[1].Status != "duplicate" || result.Items[2].Status != "registered" {
		t.Fatalf("逐项结果错误: %#v", result.Items)
	}
	reused, err := service.RegisterEvidenceBatch(collection.CollectionID, command)
	if err != nil || reused.Fingerprint != result.Fingerprint {
		t.Fatalf("批量幂等复用失败: %v", err)
	}
	view, err := service.GetCollection(collection.CollectionID, "seat-a")
	if err != nil || len(view.Evidence) != 2 {
		t.Fatalf("证据投影错误: %v count=%d", err, len(view.Evidence))
	}
	changed := command
	changed.Items = append([]BatchEvidenceItem(nil), command.Items...)
	changed.Items[2].Payload = append(changed.Items[2].Payload, 0x01)
	if _, err := service.RegisterEvidenceBatch(collection.CollectionID, changed); domain.ErrorCodeOf(err) != domain.CodeIdempotencyConflict {
		t.Fatalf("同键不同选择集应冲突，实际 %v", err)
	}
}

func TestSeatProgressAndQualityRunReuse(t *testing.T) {
	service := testService(t)
	collection := createExtensionCollection(t, service)
	captured := collection.CapturedFrom.Add(time.Hour)
	jpeg := []byte{0xff, 0xd8, 0xff, 0xd9}
	batch, err := service.RegisterEvidenceBatch(collection.CollectionID, RegisterEvidenceBatchCommand{CommandMeta: CommandMeta{Actor: "organizer", Role: RoleOrganizer, ExpectedVersion: collection.Version, IdempotencyKey: "progress-batch"}, Items: []BatchEvidenceItem{{ClientItemID: "one", OriginalName: "one.jpg", CapturedAt: captured, CameraSite: "CAM-1", PixelWidth: 10, PixelHeight: 10, Payload: jpeg}}})
	if err != nil {
		t.Fatal(err)
	}
	evidenceID := batch.Items[0].Evidence.EvidenceID
	annotation := SubmitAnnotationCommand{CommandMeta: CommandMeta{Actor: "seat-a", Role: RoleAnnotator, ExpectedVersion: batch.Version, IdempotencyKey: "seat-a-1"}, EvidenceID: evidenceID, SeatID: "seat-a", SpeciesCode: "ursus", IndividualCount: 1, Confidence: .9, Identifiability: "identifiable"}
	if _, err := service.SubmitAnnotation(collection.CollectionID, annotation); err != nil {
		t.Fatal(err)
	}
	viewB, err := service.GetCollectionWithQuery(collection.CollectionID, CollectionQuery{ViewerSeat: "seat-b", TaskFilter: "pending"})
	if err != nil || viewB.SeatProgress.Submitted != 0 || viewB.SeatProgress.Pending != 1 || len(viewB.Evidence[0].Annotations) != 0 {
		t.Fatalf("盲标进度或隔离错误: %#v %v", viewB.SeatProgress, err)
	}
	runCommand := RecalculateQualityCommand{CommandMeta: CommandMeta{Actor: "expert", Role: RoleExpert, ExpectedVersion: batch.Version + 1, IdempotencyKey: "quality-1"}}
	run, err := service.RecalculateQuality(collection.CollectionID, runCommand)
	if err != nil {
		t.Fatal(err)
	}
	reused, err := service.RecalculateQuality(collection.CollectionID, runCommand)
	if err != nil || reused.RunID != run.RunID {
		t.Fatalf("质检命令幂等复用失败: %v", err)
	}
}

func TestRemediationRequiresTargetAndBaseline(t *testing.T) {
	service := testService(t)
	collection := createExtensionCollection(t, service)
	jpeg := []byte{0xff, 0xd8, 0xff, 0xd9}
	batch, err := service.RegisterEvidenceBatch(collection.CollectionID, RegisterEvidenceBatchCommand{CommandMeta: CommandMeta{Actor: "organizer", Role: RoleOrganizer, ExpectedVersion: collection.Version, IdempotencyKey: "remediation-batch"}, Items: []BatchEvidenceItem{{ClientItemID: "one", OriginalName: "one.jpg", CapturedAt: collection.CapturedFrom.Add(time.Hour), CameraSite: "CAM-1", PixelWidth: 10, PixelHeight: 10, Payload: jpeg}}})
	if err != nil {
		t.Fatal(err)
	}
	evidenceID := batch.Items[0].Evidence.EvidenceID
	a, err := service.SubmitAnnotation(collection.CollectionID, SubmitAnnotationCommand{CommandMeta: CommandMeta{Actor: "seat-a", Role: RoleAnnotator, ExpectedVersion: batch.Version, IdempotencyKey: "rem-a"}, EvidenceID: evidenceID, SeatID: "seat-a", SpeciesCode: "ursus", IndividualCount: 1, Confidence: .9, Identifiability: "identifiable"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := service.SubmitAnnotation(collection.CollectionID, SubmitAnnotationCommand{CommandMeta: CommandMeta{Actor: "seat-b", Role: RoleAnnotator, ExpectedVersion: batch.Version + 1, IdempotencyKey: "rem-b"}, EvidenceID: evidenceID, SeatID: "seat-b", SpeciesCode: "vulpes", IndividualCount: 1, Confidence: .9, Identifiability: "identifiable"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Adjudicate(collection.CollectionID, AdjudicateCommand{CommandMeta: CommandMeta{Actor: "expert", Role: RoleExpert, ExpectedVersion: batch.Version + 2, IdempotencyKey: "request-rem"}, EvidenceID: evidenceID, Action: "request_remediation", TargetSeatID: "seat-b", Rationale: "请重新核对胸斑"})
	if err != nil {
		t.Fatal(err)
	}
	wrongSeat := SubmitAnnotationCommand{CommandMeta: CommandMeta{Actor: "seat-a", Role: RoleAnnotator, ExpectedVersion: batch.Version + 3, IdempotencyKey: "wrong-seat"}, EvidenceID: evidenceID, SeatID: "seat-a", SpeciesCode: "ursus", IndividualCount: 1, Confidence: .95, Identifiability: "identifiable", SupersedesRevisionID: a.RevisionID}
	if _, err := service.SubmitAnnotation(collection.CollectionID, wrongSeat); domain.ErrorCodeOf(err) != domain.CodeForbidden {
		t.Fatalf("非目标席位应拒绝，实际 %v", err)
	}
	correct := SubmitAnnotationCommand{CommandMeta: CommandMeta{Actor: "seat-b", Role: RoleAnnotator, ExpectedVersion: batch.Version + 3, IdempotencyKey: "correct-rem"}, EvidenceID: evidenceID, SeatID: "seat-b", SpeciesCode: "ursus", IndividualCount: 1, Confidence: .95, Identifiability: "identifiable", SupersedesRevisionID: b.RevisionID}
	returned, err := service.SubmitAnnotation(collection.CollectionID, correct)
	if err != nil {
		t.Fatal(err)
	}
	view, err := service.GetCollection(collection.CollectionID, "seat-b")
	if err != nil || view.Collection.Status != domain.StatusReview || len(view.RemediationTasks) != 1 || view.RemediationTasks[0].ResultRevisionID != returned.RevisionID {
		t.Fatalf("整改闭环错误: status=%s tasks=%#v err=%v", view.Collection.Status, view.RemediationTasks, err)
	}
}
