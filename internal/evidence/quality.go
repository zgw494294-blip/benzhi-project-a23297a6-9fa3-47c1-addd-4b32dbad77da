package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"wildframe/internal/domain"
)

const (
	RuleMetadataTime  = "META_TIME_BOUNDARY"
	RuleMetadataSite  = "META_SITE_BOUNDARY"
	RuleSeatMissing   = "ANNOTATION_SEAT_MISSING"
	RuleSpecies       = "ANNOTATION_SPECIES_REQUIRED"
	RuleCount         = "ANNOTATION_COUNT_RANGE"
	RuleLowConfidence = "ANNOTATION_LOW_CONFIDENCE"
	RuleReason        = "ANNOTATION_UNIDENTIFIABLE_REASON"
	RuleDisagreement  = "ANNOTATION_SEAT_DISAGREEMENT"
)

type QualityEngine struct {
	LowConfidence float64
	Now           func() time.Time
}

func NewQualityEngine() *QualityEngine {
	return &QualityEngine{LowConfidence: 0.60, Now: time.Now}
}

func (q *QualityEngine) Evaluate(collection domain.ImageCollection, items []domain.ImageEvidence, latest map[string]map[string]domain.AnnotationRevision) []domain.QualityFinding {
	all := make([]domain.QualityFinding, 0)
	for _, item := range items {
		all = append(all, q.EvaluateEvidence(collection, item, latest[item.EvidenceID])...)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].EvidenceID == all[j].EvidenceID {
			return all[i].RuleCode < all[j].RuleCode
		}
		return all[i].EvidenceID < all[j].EvidenceID
	})
	return all
}

func (q *QualityEngine) EvaluateEvidence(collection domain.ImageCollection, item domain.ImageEvidence, seats map[string]domain.AnnotationRevision) []domain.QualityFinding {
	now := q.Now().UTC()
	result := make([]domain.QualityFinding, 0, 8)
	add := func(rule string, severity domain.Severity, details string, revisions ...string) {
		result = append(result, makeFinding(collection.CollectionID, item.EvidenceID, rule, severity, details, revisions, now))
	}
	if item.CapturedAt.Before(collection.CapturedFrom) || item.CapturedAt.After(collection.CapturedTo) {
		add(RuleMetadataTime, domain.SeverityBlocking, "采集时间超出批次拍摄时段")
	}
	if item.CameraSite != collection.CameraSite {
		add(RuleMetadataSite, domain.SeverityBlocking, "相机点位与批次不一致")
	}
	for _, seatID := range collection.AnnotatorSeats {
		revision, ok := seats[seatID]
		if !ok {
			add(RuleSeatMissing+"_"+seatID, domain.SeverityBlocking, "席位 "+seatID+" 尚未提交")
			continue
		}
		if revision.Identifiability == "identifiable" && strings.TrimSpace(revision.SpeciesCode) == "" {
			add(RuleSpecies+"_"+seatID, domain.SeverityBlocking, "可辨识影像缺少物种代码", revision.RevisionID)
		}
		if revision.IndividualCount < 0 || revision.IndividualCount > 999 {
			add(RuleCount+"_"+seatID, domain.SeverityBlocking, "个体数量超出 0 到 999", revision.RevisionID)
		}
		if revision.Confidence < q.LowConfidence {
			add(RuleLowConfidence+"_"+seatID, domain.SeverityWarning, fmt.Sprintf("置信度 %.2f 低于 %.2f", revision.Confidence, q.LowConfidence), revision.RevisionID)
		}
		if revision.Identifiability == "unidentifiable" && strings.TrimSpace(revision.UnidentifiableReason) == "" {
			add(RuleReason+"_"+seatID, domain.SeverityBlocking, "无法辨识但未说明原因", revision.RevisionID)
		}
	}
	a, aOK := seats[collection.AnnotatorSeats[0]]
	b, bOK := seats[collection.AnnotatorSeats[1]]
	if aOK && bOK && (a.SpeciesCode != b.SpeciesCode || a.IndividualCount != b.IndividualCount || a.Identifiability != b.Identifiability) {
		add(RuleDisagreement, domain.SeverityBlocking, "两个席位的物种、数量或可辨识性不一致", a.RevisionID, b.RevisionID)
	}
	return result
}

func makeFinding(collectionID, evidenceID, rule string, severity domain.Severity, details string, revisions []string, now time.Time) domain.QualityFinding {
	sort.Strings(revisions)
	sum := sha256.Sum256([]byte(collectionID + "\x00" + evidenceID + "\x00" + rule + "\x00" + strings.Join(revisions, ",")))
	return domain.QualityFinding{
		FindingID: hex.EncodeToString(sum[:12]), CollectionID: collectionID, EvidenceID: evidenceID,
		RuleCode: rule, Severity: severity, Status: domain.FindingOpen, Details: details,
		SourceRevisionIDs: revisions, OpenedAt: now,
	}
}

func BlockingCount(findings []domain.QualityFinding) int {
	count := 0
	for _, finding := range findings {
		if finding.Status == domain.FindingOpen && finding.Severity == domain.SeverityBlocking {
			count++
		}
	}
	return count
}

func MergeAffected(previous []domain.QualityFinding, evidenceID string, recalculated []domain.QualityFinding, now time.Time) []domain.QualityFinding {
	next := make([]domain.QualityFinding, 0, len(previous)+len(recalculated))
	active := make(map[string]domain.QualityFinding, len(recalculated))
	for _, finding := range recalculated {
		active[finding.FindingID] = finding
	}
	for _, old := range previous {
		if old.EvidenceID != evidenceID {
			next = append(next, old)
			continue
		}
		if old.Status == domain.FindingResolved {
			next = append(next, old)
			continue
		}
		_, remains := active[old.FindingID]
		if !remains && old.Status == domain.FindingOpen {
			old.Status = domain.FindingResolved
			resolved := now.UTC()
			old.ResolvedAt = &resolved
			next = append(next, old)
		} else if remains && old.Status == domain.FindingOpen {
			next = append(next, old)
			delete(active, old.FindingID)
		}
	}
	for _, finding := range recalculated {
		if _, ok := active[finding.FindingID]; ok {
			next = append(next, finding)
		}
	}
	return next
}
