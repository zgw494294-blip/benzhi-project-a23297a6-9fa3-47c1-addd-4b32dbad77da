package failedadjudication_test

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
	"wildframe/internal/domain"
	"wildframe/internal/evidence"
	"wildframe/internal/persistence"
	"wildframe/internal/web"
)

func TestFailedAdjudicationDoesNotMutateProjection(t *testing.T) {
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

	const collectionID = "collection-failed-adjudication"
	const evidenceID = "evidence-failed-adjudication"
	const findingID = "finding-failed-adjudication"
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	err = repository.Update("test.awaiting_adjudication", collectionID, now, func(state *persistence.State) error {
		state.Collections[collectionID] = domain.ImageCollection{
			CollectionID: collectionID, AnnotatorSeats: [2]string{"seat-a", "seat-b"},
			Status: domain.StatusArbitration, Version: 3, CreatedAt: now, UpdatedAt: now,
		}
		state.Evidence[collectionID] = []domain.ImageEvidence{{
			EvidenceID: evidenceID, CollectionID: collectionID, RegisteredAt: now,
		}}
		state.Findings[collectionID] = []domain.QualityFinding{{
			FindingID: findingID, CollectionID: collectionID, EvidenceID: evidenceID,
			RuleCode: evidence.RuleDisagreement, Severity: domain.SeverityBlocking,
			Status: domain.FindingOpen, OpenedAt: now,
		}}
		state.Decisions[collectionID] = make(map[string]domain.AdjudicationDecision)
		return nil
	})
	if err != nil {
		t.Fatalf("准备待仲裁投影: %v", err)
	}

	ledgerPath := filepath.Join(dataDirectory, "events.jsonl")
	if err := os.Remove(ledgerPath); err != nil {
		t.Fatalf("移除测试账本文件: %v", err)
	}
	if err := os.Mkdir(ledgerPath, 0o700); err != nil {
		t.Fatalf("制造账本资源失效: %v", err)
	}

	service := application.NewService(repository, blobs, evidence.NewQualityEngine(), signer)
	handler := web.NewServer(service).Handler()
	command := application.AdjudicateCommand{
		CommandMeta: application.CommandMeta{
			Actor: "expert-1", Role: application.RoleExpert,
			ExpectedVersion: 3, IdempotencyKey: "failed-adjudication-1",
		},
		EvidenceID: evidenceID, Action: "new_conclusion",
		FinalSpeciesCode: "ursus-thibetanus", FinalCount: 1,
		Rationale: "依据胸斑完成专家裁决",
	}
	payload, err := json.Marshal(command)
	if err != nil {
		t.Fatalf("编码仲裁命令: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/collections/"+collectionID+"/adjudications", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code < http.StatusInternalServerError {
		t.Fatalf("账本追加失败未向调用方报告服务错误: status=%d body=%s", response.Code, response.Body.String())
	}

	getRequest := httptest.NewRequest(http.MethodGet, "/api/v1/collections/"+collectionID, nil)
	getResponse := httptest.NewRecorder()
	handler.ServeHTTP(getResponse, getRequest)
	if getResponse.Code != http.StatusOK {
		t.Fatalf("读取失败后的投影: status=%d body=%s", getResponse.Code, getResponse.Body.String())
	}
	var view application.CollectionView
	if err := json.Unmarshal(getResponse.Body.Bytes(), &view); err != nil {
		t.Fatalf("解码失败后的投影: %v", err)
	}
	findingStatus := domain.FindingStatus("")
	if len(view.Findings) == 1 {
		findingStatus = view.Findings[0].Status
	}
	if len(view.Decisions) != 0 || findingStatus != domain.FindingOpen || view.Collection.Version != 3 {
		t.Fatalf("失败事务污染已提交投影: decisions=%d findingStatus=%s version=%d", len(view.Decisions), findingStatus, view.Collection.Version)
	}
}
