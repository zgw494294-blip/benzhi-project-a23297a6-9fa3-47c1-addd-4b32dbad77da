package ledger_failure_poisons_snapshot_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"wildframe/internal/application"
	"wildframe/internal/evidence"
	"wildframe/internal/persistence"
	"wildframe/internal/web"
)

func TestLedgerAppendFailureDoesNotPoisonRestart(t *testing.T) {
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
	service.SetClock(func() time.Time { return time.Date(2026, 8, 27, 11, 0, 0, 0, time.UTC) })
	handler := web.NewServer(service).Handler()

	first := postCollection(t, handler, "first-create", "高黎贡山保护区")
	if first.Code != http.StatusCreated {
		t.Fatalf("首次建档失败: status=%d body=%s", first.Code, first.Body.String())
	}

	ledgerPath := filepath.Join(directory, "events.jsonl")
	backupPath := filepath.Join(directory, "events.before-failure")
	if err := os.Rename(ledgerPath, backupPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(ledgerPath, 0o700); err != nil {
		t.Fatal(err)
	}
	ledgerBlocked := true
	defer func() {
		if ledgerBlocked {
			_ = os.Remove(ledgerPath)
			_ = os.Rename(backupPath, ledgerPath)
		}
	}()

	failed := postCollection(t, handler, "second-create", "横断山保护区")
	if failed.Code != http.StatusInternalServerError {
		t.Fatalf("测试前提失败：账本资源失效时应拒绝请求，实际 status=%d body=%s", failed.Code, failed.Body.String())
	}

	if err := os.Remove(ledgerPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(backupPath, ledgerPath); err != nil {
		t.Fatal(err)
	}
	ledgerBlocked = false

	reopened, err := persistence.Open(directory)
	if err != nil {
		t.Fatalf("失败请求把未入账投影发布为快照，导致重启恢复失败: %v", err)
	}
	collectionCount := 0
	if err := reopened.Read(func(state persistence.State) error {
		collectionCount = len(state.Collections)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if collectionCount != 1 {
		t.Fatalf("失败建档不应在恢复后出现，实际批次数=%d", collectionCount)
	}
}

func postCollection(t *testing.T, handler http.Handler, idempotencyKey, reserveName string) *httptest.ResponseRecorder {
	t.Helper()
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	command := application.CreateCollectionCommand{
		CommandMeta: application.CommandMeta{Actor: "organizer", Role: application.RoleOrganizer, IdempotencyKey: idempotencyKey},
		ReserveName: reserveName, CameraSite: "CAM-LEDGER-1", CapturedFrom: from, CapturedTo: from.Add(24 * time.Hour),
		RuleSetVersion: "rules-v1", SeatA: "seat-a", SeatB: "seat-b",
	}
	body, err := json.Marshal(command)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/collections", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
