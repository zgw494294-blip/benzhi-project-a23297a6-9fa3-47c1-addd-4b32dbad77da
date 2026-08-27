package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wildframe/internal/application"
	"wildframe/internal/evidence"
	"wildframe/internal/persistence"
)

func testHandler(t *testing.T) http.Handler {
	t.Helper()
	repo, err := persistence.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := persistence.NewBlobStore(repo.Directory())
	if err != nil {
		t.Fatal(err)
	}
	signer, err := evidence.NewSigner(nil)
	if err != nil {
		t.Fatal(err)
	}
	return NewServer(application.NewService(repo, blobs, evidence.NewQualityEngine(), signer)).Handler()
}

func TestWorkbenchAndHealth(t *testing.T) {
	handler := testHandler(t)
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "<body>") {
		t.Fatalf("工作台响应无效: %d", response.Code)
	}
	if response.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("缺少安全响应头")
	}
	request = httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "ledgerHead") {
		t.Fatalf("健康响应无效: %s", response.Body.String())
	}
}

func TestJSONContentTypeBoundary(t *testing.T) {
	handler := testHandler(t)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/collections", strings.NewReader("{}"))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "VALIDATION_ERROR") {
		t.Fatalf("内容类型边界未生效: %d %s", response.Code, response.Body.String())
	}
}
