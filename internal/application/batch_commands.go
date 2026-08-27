package application

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"wildframe/internal/domain"
	"wildframe/internal/persistence"
)

const MaxBatchItems = 64

type normalizedBatchItem struct {
	input     BatchEvidenceItem
	digest    string
	mediaType string
}

func (s *Service) RegisterEvidenceBatch(collectionID string, command RegisterEvidenceBatchCommand) (RegisterEvidenceBatchResult, error) {
	if err := require(command.CommandMeta, RoleOrganizer); err != nil {
		return RegisterEvidenceBatchResult{}, err
	}
	if len(command.Items) == 0 || len(command.Items) > MaxBatchItems {
		return RegisterEvidenceBatchResult{}, domain.NewError(domain.CodeValidation, "批量影像数量必须在 1 到 %d 之间", MaxBatchItems)
	}
	canonical := struct {
		CollectionID    string           `json:"collectionID"`
		Actor           string           `json:"actor"`
		ExpectedVersion int64            `json:"expectedVersion"`
		Items           []map[string]any `json:"items"`
	}{CollectionID: collectionID, Actor: command.Actor, ExpectedVersion: command.ExpectedVersion}
	normalized := make([]normalizedBatchItem, len(command.Items))
	for index, item := range command.Items {
		sum := sha256.Sum256(item.Payload)
		digest := hex.EncodeToString(sum[:])
		mediaType := ""
		if len(item.Payload) > 0 {
			mediaType = http.DetectContentType(item.Payload)
		}
		normalized[index] = normalizedBatchItem{input: item, digest: digest, mediaType: mediaType}
		canonical.Items = append(canonical.Items, map[string]any{"clientItemID": item.ClientItemID, "originalName": item.OriginalName, "capturedAt": item.CapturedAt.UTC(), "cameraSite": item.CameraSite, "pixelWidth": item.PixelWidth, "pixelHeight": item.PixelHeight, "digest": digest, "size": len(item.Payload)})
	}
	fingerprint, err := persistence.Fingerprint(canonical)
	if err != nil {
		return RegisterEvidenceBatchResult{}, err
	}
	result := RegisterEvidenceBatchResult{CollectionID: collectionID, Fingerprint: fingerprint, Items: make([]BatchEvidenceItemResult, len(command.Items))}
	now := s.now().UTC()
	scope := "register-batch:" + collectionID
	err = s.repo.Update("evidence.batch_registered", collectionID, now, func(state *persistence.State) error {
		if reused, reuseErr := persistence.Reuse(*state, scope, command.IdempotencyKey, fingerprint, &result); reuseErr != nil || reused {
			return reuseErr
		}
		collection, collectionErr := collectionOf(*state, collectionID)
		if collectionErr != nil {
			return collectionErr
		}
		if !collection.Status.Mutable() || collection.Status != domain.StatusAnnotating {
			return domain.NewError(domain.CodeStateConflict, "已冻结、已发布或非标注中批次不接收影像")
		}
		if versionErr := domain.CheckVersion(collection, command.ExpectedVersion); versionErr != nil {
			for index, item := range command.Items {
				result.Items[index] = batchFailure(item, "version_expired", string(domain.CodeVersionConflict), versionErr.Error())
			}
			result.Version = collection.Version
			return persistence.Remember(state, scope, command.IdempotencyKey, fingerprint, result)
		}
		existing := map[string]bool{}
		for _, item := range state.Evidence[collectionID] {
			existing[item.SHA256Digest] = true
		}
		selected := map[string]bool{}
		registered := make([]domain.ImageEvidence, 0, len(normalized))
		for index, item := range normalized {
			if strings.TrimSpace(item.input.ClientItemID) == "" {
				item.input.ClientItemID = fmt.Sprintf("item-%d", index+1)
			}
			if len(item.input.Payload) == 0 || int64(len(item.input.Payload)) > persistence.MaxBlobBytes {
				result.Items[index] = batchFailure(item.input, "invalid_payload", string(domain.CodeValidation), fmt.Sprintf("影像载荷必须在 1 到 %d 字节之间", persistence.MaxBlobBytes))
				continue
			}
			if item.mediaType != "image/jpeg" && item.mediaType != "image/png" && item.mediaType != "image/webp" {
				result.Items[index] = batchFailure(item.input, "invalid_payload", string(domain.CodeValidation), "真实媒体类型不受支持")
				continue
			}
			if existing[item.digest] || selected[item.digest] {
				result.Items[index] = batchFailure(item.input, "duplicate", string(domain.CodeDuplicateEvidence), "批次或本次选择集中已存在相同摘要的证据")
				continue
			}
			candidate := domain.ImageEvidence{EvidenceID: newID("ev"), CollectionID: collectionID, OriginalName: item.input.OriginalName, CapturedAt: item.input.CapturedAt.UTC(), CameraSite: item.input.CameraSite, SHA256Digest: item.digest, MediaType: item.mediaType, ByteSize: int64(len(item.input.Payload)), PixelWidth: item.input.PixelWidth, PixelHeight: item.input.PixelHeight, BlobKey: item.digest, RegisteredAt: now}
			if validateErr := domain.ValidateEvidence(collection, candidate); validateErr != nil {
				result.Items[index] = batchFailure(item.input, "invalid_payload", string(domain.ErrorCodeOf(validateErr)), validateErr.Error())
				continue
			}
			blobKey, size, saveErr := s.blobs.Save(item.digest, item.mediaType, bytes.NewReader(item.input.Payload))
			if saveErr != nil {
				result.Items[index] = batchFailure(item.input, "retryable_storage_error", string(domain.CodeRetryableStorage), "载荷暂时无法保存，可重试")
				continue
			}
			candidate.BlobKey, candidate.ByteSize = blobKey, size
			selected[item.digest] = true
			registered = append(registered, candidate)
			copy := candidate
			result.Items[index] = BatchEvidenceItemResult{ClientItemID: item.input.ClientItemID, OriginalName: item.input.OriginalName, Status: "registered", Message: "登记成功", Evidence: &copy}
		}
		if len(registered) > 0 {
			from := collection.Status
			state.Evidence[collectionID] = append(state.Evidence[collectionID], registered...)
			domain.Touch(&collection, now)
			state.Collections[collectionID] = collection
			ids := make([]string, 0, len(registered))
			for _, item := range registered {
				ids = append(ids, item.EvidenceID)
			}
			sort.Strings(ids)
			addAuditObject(state, collection, "evidence.batch_registered", command.Actor, fmt.Sprintf("批量登记 %d 幅影像", len(registered)), from, now, "evidence_batch", strings.Join(ids, ","))
		}
		result.Version = collection.Version
		return persistence.Remember(state, scope, command.IdempotencyKey, fingerprint, result)
	})
	return result, err
}

func batchFailure(item BatchEvidenceItem, status, code, message string) BatchEvidenceItemResult {
	return BatchEvidenceItemResult{ClientItemID: item.ClientItemID, OriginalName: item.OriginalName, Status: status, Code: code, Message: message}
}
