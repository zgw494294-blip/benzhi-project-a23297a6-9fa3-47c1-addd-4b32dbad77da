package application

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"wildframe/internal/domain"
	"wildframe/internal/evidence"
	"wildframe/internal/persistence"
)

type Service struct {
	repo    *persistence.Repository
	blobs   *persistence.BlobStore
	quality *evidence.QualityEngine
	signer  *evidence.Signer
	now     func() time.Time

	viewMu    sync.RWMutex
	viewCache map[string]CollectionView
}

func NewService(repo *persistence.Repository, blobs *persistence.BlobStore, quality *evidence.QualityEngine, signer *evidence.Signer) *Service {
	return &Service{
		repo:      repo,
		blobs:     blobs,
		quality:   quality,
		signer:    signer,
		now:       time.Now,
		viewCache: make(map[string]CollectionView),
	}
}

func (s *Service) SetClock(now func() time.Time) {
	s.now = now
	s.quality.Now = now
}

func newID(prefix string) string {
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		panic(err)
	}
	return prefix + "_" + hex.EncodeToString(raw)
}

func require(meta CommandMeta, role Role) error {
	if strings.TrimSpace(meta.Actor) == "" {
		return domain.NewError(domain.CodeValidation, "操作者不能为空")
	}
	if meta.Role != role {
		return domain.NewError(domain.CodeForbidden, "该操作要求角色 %s", role)
	}
	if strings.TrimSpace(meta.IdempotencyKey) == "" || len(meta.IdempotencyKey) > 128 {
		return domain.NewError(domain.CodeValidation, "idempotencyKey 不能为空且不能超过 128 字符")
	}
	return nil
}

func collectionOf(state persistence.State, id string) (domain.ImageCollection, error) {
	collection, ok := state.Collections[id]
	if !ok {
		return domain.ImageCollection{}, domain.NewError(domain.CodeNotFound, "批次不存在")
	}
	return collection, nil
}

func evidenceOf(state persistence.State, collectionID, evidenceID string) (domain.ImageEvidence, error) {
	for _, item := range state.Evidence[collectionID] {
		if item.EvidenceID == evidenceID {
			return item, nil
		}
	}
	return domain.ImageEvidence{}, domain.NewError(domain.CodeNotFound, "影像证据不存在")
}

func latestRevisions(state persistence.State, collection domain.ImageCollection) map[string]map[string]domain.AnnotationRevision {
	latest := make(map[string]map[string]domain.AnnotationRevision)
	for _, revision := range state.Revisions[collection.CollectionID] {
		if latest[revision.EvidenceID] == nil {
			latest[revision.EvidenceID] = make(map[string]domain.AnnotationRevision)
		}
		current, ok := latest[revision.EvidenceID][revision.SeatID]
		if !ok || !current.SubmittedAt.After(revision.SubmittedAt) {
			latest[revision.EvidenceID][revision.SeatID] = revision
		}
	}
	return latest
}

func addAuditObject(state *persistence.State, collection domain.ImageCollection, action, actor, details string, from domain.CollectionStatus, now time.Time, objectType, objectID string) {
	records := state.Audits[collection.CollectionID]
	record, err := domain.NewAuditRecord(uint64(len(records)+1), collection.CollectionID, action, actor, details, from, collection.Status, now)
	if err != nil {
		panic(err)
	}
	record.ObjectType, record.ObjectID = objectType, objectID
	state.Audits[collection.CollectionID] = append(records, record)
}

func reusable(state persistence.State, scope, key string, command, result any) (bool, string, error) {
	fingerprint, err := persistence.Fingerprint(command)
	if err != nil {
		return false, "", err
	}
	ok, err := persistence.Reuse(state, scope, key, fingerprint, result)
	return ok, fingerprint, err
}

func remember(state *persistence.State, scope, key, fingerprint string, result any) error {
	if err := persistence.Remember(state, scope, key, fingerprint, result); err != nil {
		return fmt.Errorf("保存幂等结果: %w", err)
	}
	return nil
}
