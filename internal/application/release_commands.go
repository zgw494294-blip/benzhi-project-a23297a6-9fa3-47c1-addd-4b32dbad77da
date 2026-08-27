package application

import (
	"encoding/json"

	"wildframe/internal/domain"
	"wildframe/internal/evidence"
	"wildframe/internal/persistence"
)

type verificationCacheKey struct {
	CredentialID    string
	CollectionID    string
	ManifestVersion int
}

func verificationKey(envelope evidence.CredentialEnvelope) verificationCacheKey {
	return verificationCacheKey{
		CredentialID: envelope.CredentialID, CollectionID: envelope.CollectionID,
		ManifestVersion: envelope.ManifestVersion,
	}
}

func (s *Service) Freeze(collectionID string, command FreezeCommand) (domain.ImageCollection, error) {
	if err := require(command.CommandMeta, RolePublisher); err != nil {
		return domain.ImageCollection{}, err
	}
	var result domain.ImageCollection
	now := s.now().UTC()
	err := s.repo.Update("manifest.frozen", collectionID, now, func(state *persistence.State) error {
		reused, fingerprint, err := reusable(*state, "freeze:"+collectionID, command.IdempotencyKey, command, &result)
		if err != nil || reused {
			return err
		}
		collection, err := collectionOf(*state, collectionID)
		if err != nil {
			return err
		}
		if collection.Status != domain.StatusFreezable {
			return domain.NewError(domain.CodeStateConflict, "批次当前不可冻结")
		}
		preview, ok := state.ManifestPreviews[collectionID]
		if !ok || preview.CollectionVersion != collection.Version || preview.CollectionVersion != command.ExpectedVersion || preview.Digest != command.ManifestDigest {
			return domain.NewError(domain.CodePreviewExpired, "清单预览已过期，请重新生成并确认")
		}
		if err := domain.CheckVersion(collection, command.ExpectedVersion); err != nil {
			return err
		}
		manifest, canonical, digest, err := s.buildManifest(*state, collection)
		if err != nil {
			return err
		}
		if command.ManifestDigest != digest || !json.Valid(preview.Canonical) {
			return domain.NewError(domain.CodePreviewExpired, "确认摘要与当前清单预览不一致")
		}
		from := collection.Status
		if err := domain.Transition(&collection, command.ExpectedVersion, domain.StatusFrozen, now); err != nil {
			return err
		}
		collection.ManifestDigest, collection.ManifestVersion = digest, manifest.SchemaVersion
		state.Collections[collectionID] = collection
		state.Manifests[collectionID] = append([]byte(nil), canonical...)
		addAuditObject(state, collection, "manifest.frozen", command.Actor, "冻结不可变清单，摘要 "+digest, from, now, "manifest", digest)
		result = collection
		return remember(state, "freeze:"+collectionID, command.IdempotencyKey, fingerprint, result)
	})
	return result, err
}

func (s *Service) IssueCredential(collectionID string, command IssueCommand) (evidence.CredentialEnvelope, error) {
	if err := require(command.CommandMeta, RolePublisher); err != nil {
		return evidence.CredentialEnvelope{}, err
	}
	var result evidence.CredentialEnvelope
	now := s.now().UTC()
	err := s.repo.Update("credential.issued", collectionID, now, func(state *persistence.State) error {
		reused, fingerprint, err := reusable(*state, "issue:"+collectionID, command.IdempotencyKey, command, &result)
		if err != nil || reused {
			return err
		}
		collection, err := collectionOf(*state, collectionID)
		if err != nil {
			return err
		}
		if err := domain.CheckVersion(collection, command.ExpectedVersion); err != nil {
			return err
		}
		if collection.Status != domain.StatusFrozen {
			return domain.NewError(domain.CodeStateConflict, "只有已冻结批次可签发凭据")
		}
		result = s.signer.Issue(newID("cred"), collectionID, collection.ManifestDigest, command.Actor, collection.ManifestVersion, now)
		from := collection.Status
		if err := domain.Transition(&collection, command.ExpectedVersion, domain.StatusReleased, now); err != nil {
			return err
		}
		state.Collections[collectionID] = collection
		raw, err := json.Marshal(result)
		if err != nil {
			return err
		}
		state.Credentials[collectionID] = raw
		addAuditObject(state, collection, "credential.issued", command.Actor, "签发 Ed25519 发布凭据", from, now, "credential", result.CredentialID)
		return remember(state, "issue:"+collectionID, command.IdempotencyKey, fingerprint, result)
	})
	return result, err
}

func (s *Service) VerifyCredential(envelope evidence.CredentialEnvelope) VerificationResult {
	cacheKey := verificationKey(envelope)
	s.verifyMu.RLock()
	cached, ok := s.verifyCache[cacheKey]
	s.verifyMu.RUnlock()
	if ok {
		return cached
	}

	valid, message := evidence.VerifyCredential(envelope, "")
	result := VerificationResult{Valid: false, SignatureValid: valid, SignatureStatus: "invalid", BindingStatus: "not_checked", Message: message, CredentialID: envelope.CredentialID, CollectionID: envelope.CollectionID, ManifestDigest: envelope.ManifestDigest, ManifestVersion: envelope.ManifestVersion, IssuedBy: envelope.IssuedBy, IssuedAt: envelope.IssuedAt}
	if !valid {
		s.rememberVerification(cacheKey, result)
		return result
	}
	result.SignatureStatus = "valid"
	err := s.repo.Read(func(state persistence.State) error {
		collection, ok := state.Collections[envelope.CollectionID]
		if !ok {
			result.BindingStatus = "collection_not_found"
			result.Message = "签名有效，但本地批次不存在"
			return nil
		}
		if collection.Status != domain.StatusFrozen && collection.Status != domain.StatusReleased {
			result.BindingStatus = "collection_not_frozen"
			result.Message = "签名有效，但本地批次尚未冻结"
			return nil
		}
		if collection.ManifestDigest != envelope.ManifestDigest || collection.ManifestVersion != envelope.ManifestVersion {
			result.BindingStatus = "manifest_mismatch"
			result.Message = "签名有效，但本地冻结清单摘要不匹配"
			return nil
		}
		result.BindingStatus = "valid"
		result.Valid = true
		result.Message = "签名有效且已绑定本地不可变清单"
		return nil
	})
	if err != nil {
		result.Valid, result.BindingStatus, result.Message = false, "read_error", "读取冻结清单失败"
	}
	s.rememberVerification(cacheKey, result)
	return result
}

func (s *Service) rememberVerification(key verificationCacheKey, result VerificationResult) {
	s.verifyMu.Lock()
	s.verifyCache[key] = result
	s.verifyMu.Unlock()
}
