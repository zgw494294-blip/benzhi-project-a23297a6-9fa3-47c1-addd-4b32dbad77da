package restartidempotency_test

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

func TestRestartPreservesCreateIdempotency(t *testing.T) {
	dataDirectory := t.TempDir()
	newHandler := func() http.Handler {
		t.Helper()
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
		return web.NewServer(application.NewService(repository, blobs, evidence.NewQualityEngine(), signer)).Handler()
	}
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	command := application.CreateCollectionCommand{
		CommandMeta: application.CommandMeta{
			Actor: "organizer-1", Role: application.RoleOrganizer,
			ExpectedVersion: 0, IdempotencyKey: "restart-create-1",
		},
		ReserveName: "重启幂等保护区", CameraSite: "CAM-RESTART-01",
		CapturedFrom: from, CapturedTo: from.Add(24 * time.Hour),
		RuleSetVersion: "rules-v1", SeatA: "seat-a", SeatB: "seat-b",
	}
	payload, err := json.Marshal(command)
	if err != nil {
		t.Fatalf("编码建档命令: %v", err)
	}
	create := func(handler http.Handler) domain.ImageCollection {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/api/v1/collections", bytes.NewReader(payload))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusCreated {
			t.Fatalf("建档响应异常: status=%d body=%s", response.Code, response.Body.String())
		}
		var collection domain.ImageCollection
		if err := json.Unmarshal(response.Body.Bytes(), &collection); err != nil {
			t.Fatalf("解码建档结果: %v", err)
		}
		return collection
	}

	firstHandler := newHandler()
	first := create(firstHandler)
	restartedHandler := newHandler()
	second := create(restartedHandler)

	listRequest := httptest.NewRequest(http.MethodGet, "/api/v1/collections", nil)
	listResponse := httptest.NewRecorder()
	restartedHandler.ServeHTTP(listResponse, listRequest)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("读取重启后批次列表: status=%d body=%s", listResponse.Code, listResponse.Body.String())
	}
	var list struct {
		Collections []domain.ImageCollection `json:"collections"`
	}
	if err := json.Unmarshal(listResponse.Body.Bytes(), &list); err != nil {
		t.Fatalf("解码批次列表: %v", err)
	}

	getRequest := httptest.NewRequest(http.MethodGet, "/api/v1/collections/"+first.CollectionID, nil)
	getResponse := httptest.NewRecorder()
	restartedHandler.ServeHTTP(getResponse, getRequest)
	if getResponse.Code != http.StatusOK {
		t.Fatalf("读取原批次: status=%d body=%s", getResponse.Code, getResponse.Body.String())
	}
	var original application.CollectionView
	if err := json.Unmarshal(getResponse.Body.Bytes(), &original); err != nil {
		t.Fatalf("解码原批次: %v", err)
	}
	if second.CollectionID != first.CollectionID || len(list.Collections) != 1 || len(original.Audit) != 1 {
		t.Fatalf("重启后幂等与审计状态丢失: first=%s second=%s collections=%d audit=%d", first.CollectionID, second.CollectionID, len(list.Collections), len(original.Audit))
	}
}
