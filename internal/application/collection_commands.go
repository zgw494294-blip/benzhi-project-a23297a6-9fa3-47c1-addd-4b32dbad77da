package application

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"

	"wildframe/internal/domain"
	"wildframe/internal/evidence"
	"wildframe/internal/persistence"
)

func (s *Service) CreateCollection(command CreateCollectionCommand) (domain.ImageCollection, error) {
	if err := require(command.CommandMeta, RoleOrganizer); err != nil {
		return domain.ImageCollection{}, err
	}
	if command.ExpectedVersion != 0 {
		return domain.ImageCollection{}, domain.NewError(domain.CodeVersionConflict, "新建批次的 expectedVersion 必须为 0")
	}
	var result domain.ImageCollection
	now := s.now().UTC()
	err := s.repo.Update("collection.created", "", now, func(state *persistence.State) error {
		reused, fingerprint, err := reusable(*state, "create", command.IdempotencyKey, command, &result)
		if err != nil || reused {
			return err
		}
		collection, err := domain.NewCollection(newID("col"), domain.CreateCollectionInput{
			ReserveName: command.ReserveName, CameraSite: command.CameraSite, CapturedFrom: command.CapturedFrom,
			CapturedTo: command.CapturedTo, RuleSetVersion: command.RuleSetVersion, SeatA: command.SeatA, SeatB: command.SeatB,
		}, now)
		if err != nil {
			return err
		}
		from := collection.Status
		if err := domain.Transition(&collection, collection.Version, domain.StatusAnnotating, now); err != nil {
			return err
		}
		state.Collections[collection.CollectionID] = collection
		addAuditObject(state, collection, "collection.created", command.Actor, "建立批次并开始独立标注", from, now, "collection", collection.CollectionID)
		result = collection
		return remember(state, "create", command.IdempotencyKey, fingerprint, result)
	})
	return result, err
}

func (s *Service) RegisterEvidence(collectionID string, command RegisterEvidenceCommand) (domain.ImageEvidence, error) {
	if err := require(command.CommandMeta, RoleOrganizer); err != nil {
		return domain.ImageEvidence{}, err
	}
	if len(command.Payload) == 0 {
		return domain.ImageEvidence{}, domain.NewError(domain.CodeValidation, "影像载荷不能为空")
	}
	actual := sha256.Sum256(command.Payload)
	actualDigest := hex.EncodeToString(actual[:])
	if command.SHA256Digest != actualDigest {
		return domain.ImageEvidence{}, domain.NewError(domain.CodeValidation, "影像载荷 SHA-256 与登记摘要不一致")
	}
	detectedType := http.DetectContentType(command.Payload)
	if detectedType != command.MediaType {
		return domain.ImageEvidence{}, domain.NewError(domain.CodeValidation, "影像真实媒体类型与登记类型不一致")
	}
	var result domain.ImageEvidence
	now := s.now().UTC()
	scope := "register:" + collectionID
	commandFingerprint := struct {
		RegisterEvidenceCommand
		PayloadSize int
	}{command, len(command.Payload)}
	fingerprint, err := persistence.Fingerprint(commandFingerprint)
	if err != nil {
		return result, err
	}
	var existing bool
	err = s.repo.Read(func(state persistence.State) error {
		var readErr error
		existing, readErr = persistence.Reuse(state, scope, command.IdempotencyKey, fingerprint, &result)
		return readErr
	})
	if err != nil || existing {
		return result, err
	}
	err = s.repo.Update("evidence.registered", collectionID, now, func(state *persistence.State) error {
		if reused, reuseErr := persistence.Reuse(*state, scope, command.IdempotencyKey, fingerprint, &result); reuseErr != nil || reused {
			return reuseErr
		}
		collection, err := collectionOf(*state, collectionID)
		if err != nil {
			return err
		}
		if err := domain.CheckVersion(collection, command.ExpectedVersion); err != nil {
			return err
		}
		for _, item := range state.Evidence[collectionID] {
			if item.SHA256Digest == command.SHA256Digest {
				return domain.NewError(domain.CodeDuplicateEvidence, "批次内已存在相同摘要的证据")
			}
		}
		item := domain.ImageEvidence{EvidenceID: newID("ev"), CollectionID: collectionID, OriginalName: command.OriginalName,
			CapturedAt: command.CapturedAt.UTC(), CameraSite: command.CameraSite, SHA256Digest: command.SHA256Digest,
			MediaType: command.MediaType, ByteSize: int64(len(command.Payload)), PixelWidth: command.PixelWidth, PixelHeight: command.PixelHeight,
			BlobKey: command.SHA256Digest, RegisteredAt: now}
		if err := domain.ValidateEvidence(collection, item); err != nil {
			return err
		}
		blobKey, byteSize, err := s.blobs.Save(command.SHA256Digest, command.MediaType, bytes.NewReader(command.Payload))
		if err != nil {
			return domain.NewError(domain.CodeValidation, "%s", err)
		}
		item.BlobKey, item.ByteSize = blobKey, byteSize
		from := collection.Status
		domain.Touch(&collection, now)
		state.Collections[collectionID] = collection
		state.Evidence[collectionID] = append(state.Evidence[collectionID], item)
		addAuditObject(state, collection, "evidence.registered", command.Actor, fmt.Sprintf("登记影像 %s，摘要 %s", item.OriginalName, item.SHA256Digest), from, now, "evidence", item.EvidenceID)
		result = item
		return persistence.Remember(state, scope, command.IdempotencyKey, fingerprint, result)
	})
	return result, err
}

func (s *Service) PreviewManifest(collectionID string) (ManifestPreview, error) {
	var preview ManifestPreview
	now := s.now().UTC()
	err := s.repo.Update("manifest.previewed", collectionID, now, func(state *persistence.State) error {
		collection, err := collectionOf(*state, collectionID)
		if err != nil {
			return err
		}
		if collection.Status != domain.StatusFreezable && collection.Status != domain.StatusFrozen && collection.Status != domain.StatusReleased {
			return domain.NewError(domain.CodeStateConflict, "专家复核通过后才能预览冻结清单")
		}
		var manifest evidence.Manifest
		var canonical []byte
		var digest string
		if collection.Status == domain.StatusFrozen || collection.Status == domain.StatusReleased {
			canonical = append([]byte(nil), state.Manifests[collectionID]...)
			if err := json.Unmarshal(canonical, &manifest); err != nil {
				return err
			}
			digest = collection.ManifestDigest
		} else {
			manifest, canonical, digest, err = s.buildManifest(*state, collection)
		}
		if err != nil {
			return err
		}
		baseline := evidence.Manifest{}
		if previous, ok := state.ManifestPreviews[collectionID]; ok && len(previous.Canonical) > 0 {
			_ = json.Unmarshal(previous.Canonical, &baseline)
		}
		preview = ManifestPreview{Manifest: manifest, Digest: digest, CollectionVersion: collection.Version, GeneratedAt: now, Statistics: evidence.SummarizeManifest(manifest), Diff: evidence.DiffManifests(baseline, manifest)}
		state.ManifestPreviews[collectionID] = domain.ManifestPreviewRecord{CollectionID: collectionID, CollectionVersion: collection.Version, Digest: digest, Canonical: append([]byte(nil), canonical...), GeneratedAt: now}
		return nil
	})
	return preview, err
}

func (s *Service) buildManifest(state persistence.State, collection domain.ImageCollection) (evidence.Manifest, []byte, string, error) {
	return evidence.BuildManifest(collection, state.Evidence[collection.CollectionID], latestRevisions(state, collection), state.Decisions[collection.CollectionID])
}

func marshal(value any) json.RawMessage { raw, _ := json.Marshal(value); return raw }
