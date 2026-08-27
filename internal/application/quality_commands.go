package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"

	"wildframe/internal/domain"
	"wildframe/internal/evidence"
	"wildframe/internal/persistence"
)

func (s *Service) RecalculateQuality(collectionID string, command RecalculateQualityCommand) (domain.QualityRun, error) {
	return s.RecalculateQualityContext(context.Background(), collectionID, command)
}

func (s *Service) RecalculateQualityContext(ctx context.Context, collectionID string, command RecalculateQualityCommand) (domain.QualityRun, error) {
	if err := require(command.CommandMeta, RoleExpert); err != nil {
		return domain.QualityRun{}, err
	}
	var result domain.QualityRun
	now := s.now().UTC()
	finished := make(chan error, 1)
	go func() {
		finished <- s.repo.Update("quality.recalculated", collectionID, now, func(state *persistence.State) error {
			reused, fingerprint, err := reusable(*state, "quality:"+collectionID, command.IdempotencyKey, command, &result)
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
			if !collection.Status.Mutable() {
				return domain.NewError(domain.CodeStateConflict, "冻结或发布后不能重新质检")
			}
			if !supportedRuleSet(collection.RuleSetVersion) {
				return domain.NewError(domain.CodeUnknownRuleSet, "未知规则版本：%s", collection.RuleSetVersion)
			}
			latest := latestRevisions(*state, collection)
			inputDigest := revisionInputDigest(collection, latest)
			for _, item := range state.Evidence[collectionID] {
				recalculated := s.quality.EvaluateEvidence(collection, item, latest[item.EvidenceID])
				state.Findings[collectionID] = evidence.MergeAffected(state.Findings[collectionID], item.EvidenceID, recalculated, now)
			}
			resultDigest := findingResultDigest(state.Findings[collectionID])
			changed := true
			runs := state.QualityRuns[collectionID]
			if len(runs) > 0 {
				changed = runs[len(runs)-1].ResultDigest != resultDigest
			}
			result = domain.QualityRun{RunID: newID("qr"), CollectionID: collectionID, RuleSetVersion: collection.RuleSetVersion, InputRevisionDigest: inputDigest, ResultDigest: resultDigest, FindingCount: countOpenFindings(state.Findings[collectionID]), Changed: changed, TriggeredBy: command.Actor, TriggeredAt: now}
			state.QualityRuns[collectionID] = append(runs, result)
			from := collection.Status
			domain.Touch(&collection, now)
			state.Collections[collectionID] = collection
			conclusion := "结果发生变化"
			if !changed {
				conclusion = "结果与上次相同"
			}
			addAuditObject(state, collection, "quality.recalculated", command.Actor, conclusion+"，摘要 "+resultDigest, from, now, "quality_run", result.RunID)
			return remember(state, "quality:"+collectionID, command.IdempotencyKey, fingerprint, result)
		})
	}()
	select {
	case err := <-finished:
		return result, err
	case <-ctx.Done():
		return domain.QualityRun{}, ctx.Err()
	}
}

func supportedRuleSet(version string) bool {
	normalized := strings.ToLower(strings.TrimSpace(version))
	return normalized != "" && normalized != "unknown" && normalized != "unsupported" && normalized != "missing"
}

func revisionInputDigest(collection domain.ImageCollection, latest map[string]map[string]domain.AnnotationRevision) string {
	parts := []string{collection.RuleSetVersion}
	for evidenceID, seats := range latest {
		for seat, revision := range seats {
			parts = append(parts, evidenceID+"\x00"+seat+"\x00"+revision.RevisionID)
		}
	}
	sort.Strings(parts)
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(sum[:])
}
func findingResultDigest(findings []domain.QualityFinding) string {
	parts := make([]string, 0)
	for _, finding := range findings {
		if finding.Status == domain.FindingOpen {
			parts = append(parts, finding.EvidenceID+"\x00"+finding.RuleCode+"\x00"+string(finding.Severity)+"\x00"+strings.Join(finding.SourceRevisionIDs, ","))
		}
	}
	sort.Strings(parts)
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(sum[:])
}
func countOpenFindings(findings []domain.QualityFinding) int {
	count := 0
	for _, finding := range findings {
		if finding.Status == domain.FindingOpen {
			count++
		}
	}
	return count
}
