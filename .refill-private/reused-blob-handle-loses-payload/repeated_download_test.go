package blobhandle_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
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

func TestRepeatedBlobDownloadReturnsCompletePayload(t *testing.T) {
	dataDirectory := t.TempDir()
	repository, err := persistence.Open(dataDirectory)
	if err != nil {
		t.Fatalf("打开测试仓库: %v", err)
	}
	blobs, err := persistence.NewBlobStore(dataDirectory)
	if err != nil {
		t.Fatalf("打开测试载荷目录: %v", err)
	}
	payload := []byte("\x89PNG\r\n\x1a\nrepeated-download-payload")
	sum := sha256.Sum256(payload)
	digest := hex.EncodeToString(sum[:])
	blobKey, _, err := blobs.Save(digest, "image/png", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("保存测试载荷: %v", err)
	}

	const collectionID = "collection-repeated-download"
	const evidenceID = "evidence-repeated-download"
	now := time.Date(2026, 8, 27, 10, 15, 0, 0, time.UTC)
	err = repository.Update("test.evidence_registered", collectionID, now, func(state *persistence.State) error {
		state.Collections[collectionID] = domain.ImageCollection{
			CollectionID: collectionID, Status: domain.StatusAnnotating,
			Version: 1, CreatedAt: now, UpdatedAt: now,
		}
		state.Evidence[collectionID] = []domain.ImageEvidence{{
			EvidenceID: evidenceID, CollectionID: collectionID, BlobKey: blobKey,
			SHA256Digest: digest, MediaType: "image/png", ByteSize: int64(len(payload)),
			RegisteredAt: now,
		}}
		return nil
	})
	if err != nil {
		t.Fatalf("准备证据投影: %v", err)
	}
	signer, err := evidence.NewSigner(nil)
	if err != nil {
		t.Fatalf("创建测试签发器: %v", err)
	}
	service := application.NewService(repository, blobs, evidence.NewQualityEngine(), signer)
	handler := web.NewServer(service).Handler()
	download := func() *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, "/api/v1/collections/"+collectionID+"/blobs/"+blobKey, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}

	first := download()
	if first.Code != http.StatusOK || !bytes.Equal(first.Body.Bytes(), payload) {
		t.Fatalf("首次下载不完整: status=%d body=%q", first.Code, first.Body.Bytes())
	}
	second := download()
	if second.Code != http.StatusOK || !bytes.Equal(second.Body.Bytes(), payload) {
		t.Fatalf("重复下载未返回完整载荷: status=%d body=%q", second.Code, second.Body.Bytes())
	}
}
