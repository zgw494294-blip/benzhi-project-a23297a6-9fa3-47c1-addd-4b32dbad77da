package credentialcache_test

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"wildframe/internal/application"
	"wildframe/internal/domain"
	"wildframe/internal/evidence"
	"wildframe/internal/persistence"
	"wildframe/internal/web"
)

func TestCredentialVerificationCacheDoesNotTrustMutatedEnvelope(t *testing.T) {
	dataDirectory := t.TempDir()
	repository, err := persistence.Open(dataDirectory)
	if err != nil {
		t.Fatalf("打开测试仓库: %v", err)
	}
	blobs, err := persistence.NewBlobStore(dataDirectory)
	if err != nil {
		t.Fatalf("打开测试载荷目录: %v", err)
	}
	seed := make([]byte, ed25519.SeedSize)
	for index := range seed {
		seed[index] = byte(index + 1)
	}
	signer, err := evidence.NewSigner(ed25519.NewKeyFromSeed(seed))
	if err != nil {
		t.Fatalf("创建测试签发器: %v", err)
	}

	const collectionID = "collection-cache-boundary"
	const credentialID = "credential-cache-boundary"
	digest := strings.Repeat("a", 64)
	now := time.Date(2026, 8, 27, 9, 30, 0, 0, time.UTC)
	err = repository.Update("test.collection_frozen", collectionID, now, func(state *persistence.State) error {
		state.Collections[collectionID] = domain.ImageCollection{
			CollectionID: collectionID, Status: domain.StatusFrozen, Version: 7,
			ManifestDigest: digest, ManifestVersion: 1, CreatedAt: now, UpdatedAt: now,
		}
		return nil
	})
	if err != nil {
		t.Fatalf("准备冻结批次: %v", err)
	}

	service := application.NewService(repository, blobs, evidence.NewQualityEngine(), signer)
	handler := web.NewServer(service).Handler()
	verify := func(envelope evidence.CredentialEnvelope) application.VerificationResult {
		t.Helper()
		payload, marshalErr := json.Marshal(envelope)
		if marshalErr != nil {
			t.Fatalf("编码凭据: %v", marshalErr)
		}
		request := httptest.NewRequest(http.MethodPost, "/api/v1/credentials/verify", bytes.NewReader(payload))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("验证接口状态码 = %d，响应 = %s", response.Code, response.Body.String())
		}
		var result application.VerificationResult
		if decodeErr := json.Unmarshal(response.Body.Bytes(), &result); decodeErr != nil {
			t.Fatalf("解码验证结果: %v", decodeErr)
		}
		return result
	}

	validEnvelope := signer.Issue(credentialID, collectionID, digest, "publisher-1", 1, now)
	if first := verify(validEnvelope); !first.Valid || !first.SignatureValid || first.BindingStatus != "valid" {
		t.Fatalf("合法凭据首次验证未通过: %+v", first)
	}

	tamperedEnvelope := validEnvelope
	tamperedEnvelope.ManifestDigest = strings.Repeat("b", 64)
	second := verify(tamperedEnvelope)
	if second.Valid || second.SignatureValid {
		t.Fatalf("篡改后的同身份凭据复用了首次验证结果: %+v", second)
	}
}
