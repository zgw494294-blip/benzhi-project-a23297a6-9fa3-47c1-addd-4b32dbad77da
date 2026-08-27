package domain

import (
	"fmt"
	"strings"
	"time"
)

// NewAuditRecord creates the domain event representation retained in the query
// projection. The persistence ledger wraps projections in its cryptographic
// sequence and hash chain.
func NewAuditRecord(sequence uint64, collectionID, action, actor, details string, from, to CollectionStatus, occurredAt time.Time) (AuditRecord, error) {
	if sequence == 0 {
		return AuditRecord{}, NewError(CodeValidation, "审计序号必须从 1 开始")
	}
	if strings.TrimSpace(collectionID) == "" || strings.TrimSpace(action) == "" {
		return AuditRecord{}, NewError(CodeValidation, "审计记录缺少批次或操作名称")
	}
	if strings.TrimSpace(actor) == "" {
		return AuditRecord{}, NewError(CodeValidation, "审计记录缺少操作者")
	}
	if occurredAt.IsZero() {
		return AuditRecord{}, NewError(CodeValidation, "审计记录缺少发生时间")
	}
	return AuditRecord{
		Sequence: sequence, CollectionID: collectionID, Action: action, Actor: actor,
		FromStatus: from, ToStatus: to, Details: details, OccurredAt: occurredAt.UTC(),
	}, nil
}

// ValidateAuditTimeline prevents a damaged projection from presenting a
// plausible but incomplete chain of custody to a publisher.
func ValidateAuditTimeline(collectionID string, records []AuditRecord) error {
	var previousTime time.Time
	for index, record := range records {
		expected := uint64(index + 1)
		if record.Sequence != expected {
			return fmt.Errorf("批次 %s 审计序号不连续：期望 %d，实际 %d", collectionID, expected, record.Sequence)
		}
		if record.CollectionID != collectionID {
			return fmt.Errorf("批次 %s 审计记录归属错误", collectionID)
		}
		if !previousTime.IsZero() && record.OccurredAt.Before(previousTime) {
			return fmt.Errorf("批次 %s 审计时间倒序", collectionID)
		}
		if !ValidStatusTransition(record.FromStatus, record.ToStatus) {
			return fmt.Errorf("批次 %s 审计包含非法状态迁移：%s → %s", collectionID, record.FromStatus, record.ToStatus)
		}
		previousTime = record.OccurredAt
	}
	return nil
}
