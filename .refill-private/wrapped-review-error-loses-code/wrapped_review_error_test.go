package wrappedreviewerror_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"wildframe/internal/application"
	"wildframe/internal/domain"
	"wildframe/internal/evidence"
	"wildframe/internal/persistence"
	"wildframe/internal/web"
)

func TestWrappedReviewConflictKeepsBusinessCode(t *testing.T) {
	dataDirectory := t.TempDir()
	repository, err := persistence.Open(dataDirectory)
	if err != nil {
		t.Fatalf("打开测试仓库: %v", err)
	}
	blobs, err := persistence.NewBlobStore(dataDirectory)
	if err != nil {
		t.Fatalf("打开测试载荷目录: %v", err)
	}
	signer, err := evidence.NewSigner(nil)
	if err != nil {
		t.Fatalf("创建测试签发器: %v", err)
	}

	const collectionID = "collection-wrapped-review"
	now := time.Date(2026, 8, 27, 13, 0, 0, 0, time.UTC)
	err = repository.Update("test.awaiting_review", collectionID, now, func(state *persistence.State) error {
		state.Collections[collectionID] = domain.ImageCollection{
			CollectionID: collectionID, Status: domain.StatusReview,
			Version: 5, CreatedAt: now, UpdatedAt: now,
		}
		return nil
	})
	if err != nil {
		t.Fatalf("准备待复核批次: %v", err)
	}

	service := application.NewService(repository, blobs, evidence.NewQualityEngine(), signer)
	handler := web.NewServer(service).Handler()
	command := application.ReviewCommand{CommandMeta: application.CommandMeta{
		Actor: "expert-1", Role: application.RoleExpert,
		ExpectedVersion: 4, IdempotencyKey: "stale-review-1",
	}}
	payload, err := json.Marshal(command)
	if err != nil {
		t.Fatalf("编码复核命令: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/collections/"+collectionID+"/review", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("解码错误响应: %v", err)
	}
	if response.Code != http.StatusConflict || body.Error.Code != string(domain.CodeVersionConflict) {
		t.Fatalf("包装后的版本冲突丢失业务语义: status=%d code=%s message=%s", response.Code, body.Error.Code, body.Error.Message)
	}
}
