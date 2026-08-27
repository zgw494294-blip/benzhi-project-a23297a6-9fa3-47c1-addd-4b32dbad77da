package application

import (
	"fmt"
	"strings"
	"time"

	"wildframe/internal/domain"
	"wildframe/internal/evidence"
	"wildframe/internal/persistence"
)

func (s *Service) Adjudicate(collectionID string, command AdjudicateCommand) (domain.AdjudicationDecision, error) {
	if err := require(command.CommandMeta, RoleExpert); err != nil {
		return domain.AdjudicationDecision{}, err
	}
	var result domain.AdjudicationDecision
	now := s.now().UTC()
	scope := "adjudicate:" + collectionID + ":" + command.EvidenceID
	err := s.repo.Update("adjudication.decided", collectionID, now, func(state *persistence.State) error {
		reused, fingerprint, err := reusable(*state, scope, command.IdempotencyKey, command, &result)
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
		if err := domain.EnsureMutable(collection); err != nil {
			return err
		}
		if collection.Status != domain.StatusArbitration {
			return domain.NewError(domain.CodeStateConflict, "仅待仲裁批次可提交专家决定")
		}
		if _, err := evidenceOf(*state, collectionID, command.EvidenceID); err != nil {
			return err
		}
		latest := latestRevisions(*state, collection)[command.EvidenceID]
		decision, validationErr := validateAdjudication(collection, command, latest, now)
		if validationErr != nil {
			return validationErr
		}
		if state.Decisions[collectionID] == nil {
			state.Decisions[collectionID] = make(map[string]domain.AdjudicationDecision)
		}
		state.Decisions[collectionID][command.EvidenceID] = decision
		state.DecisionHistory[collectionID] = append(state.DecisionHistory[collectionID], decision)
		from := collection.Status
		if command.Action == "request_remediation" {
			for _, task := range state.RemediationTasks[collectionID] {
				if task.EvidenceID == command.EvidenceID && task.Status == "open" {
					return domain.NewError(domain.CodeStateConflict, "该证据已有开放整改任务")
				}
			}
			baseline := latest[command.TargetSeatID]
			if baseline.RevisionID == "" {
				return domain.NewError(domain.CodeValidation, "整改目标席位尚无可替代的基线修订")
			}
			round := 1
			for _, task := range state.RemediationTasks[collectionID] {
				if task.EvidenceID == command.EvidenceID && task.Round >= round {
					round = task.Round + 1
				}
			}
			task := domain.RemediationTask{TaskID: newID("rem"), CollectionID: collectionID, EvidenceID: command.EvidenceID, Round: round, TargetSeatID: command.TargetSeatID, Rationale: command.Rationale, BaselineRevisionID: baseline.RevisionID, Status: "open", CreatedAt: now}
			state.RemediationTasks[collectionID] = append(state.RemediationTasks[collectionID], task)
			collection.Status = domain.StatusRemediation
		} else {
			state.Findings[collectionID] = resolveEvidenceFindings(state.Findings[collectionID], command.EvidenceID, now)
			if evidence.BlockingCount(state.Findings[collectionID]) == 0 {
				collection.Status = domain.StatusReview
			}
		}
		domain.Touch(&collection, now)
		state.Collections[collectionID] = collection
		addAuditObject(state, collection, "adjudication.decided", command.Actor, fmt.Sprintf("证据 %s：%s", command.EvidenceID, command.Action), from, now, "decision", decision.DecisionID)
		result = decision
		return remember(state, scope, command.IdempotencyKey, fingerprint, result)
	})
	return result, wrapUseCase("提交仲裁", err)
}

func validateAdjudication(collection domain.ImageCollection, command AdjudicateCommand, latest map[string]domain.AnnotationRevision, now time.Time) (domain.AdjudicationDecision, error) {
	if strings.TrimSpace(command.Rationale) == "" {
		return domain.AdjudicationDecision{}, domain.NewError(domain.CodeValidation, "仲裁理由不能为空")
	}
	decision := domain.AdjudicationDecision{DecisionID: newID("dec"), CollectionID: collection.CollectionID, EvidenceID: command.EvidenceID, Action: command.Action, Rationale: command.Rationale, TargetSeatID: command.TargetSeatID, ReviewerID: command.Actor, DecidedAt: now}
	switch command.Action {
	case "adopt_seat":
		if !domain.HasSeat(collection, command.TargetSeatID) {
			return decision, domain.NewError(domain.CodeValidation, "采纳席位无效")
		}
		chosen := latest[command.TargetSeatID]
		if chosen.RevisionID == "" {
			return decision, domain.NewError(domain.CodeValidation, "目标席位尚未提交")
		}
		decision.FinalSpeciesCode, decision.FinalCount = chosen.SpeciesCode, chosen.IndividualCount
	case "new_conclusion":
		if strings.TrimSpace(command.FinalSpeciesCode) == "" || command.FinalCount < 0 || command.FinalCount > 999 {
			return decision, domain.NewError(domain.CodeValidation, "专家新结论的物种或数量无效")
		}
		decision.FinalSpeciesCode, decision.FinalCount = command.FinalSpeciesCode, command.FinalCount
	case "request_remediation":
		if !domain.HasSeat(collection, command.TargetSeatID) {
			return decision, domain.NewError(domain.CodeValidation, "整改目标席位无效")
		}
		if latest[command.TargetSeatID].RevisionID == "" {
			return decision, domain.NewError(domain.CodeValidation, "整改目标席位尚未提交")
		}
	default:
		return decision, domain.NewError(domain.CodeValidation, "仲裁 action 无效")
	}
	return decision, nil
}

func (s *Service) PreviewAdjudication(collectionID string, command AdjudicateCommand) (AdjudicationPreview, error) {
	if err := require(command.CommandMeta, RoleExpert); err != nil {
		return AdjudicationPreview{}, err
	}
	var result AdjudicationPreview
	err := s.repo.Read(func(state persistence.State) error {
		collection, err := collectionOf(state, collectionID)
		if err != nil {
			return err
		}
		if err := domain.CheckVersion(collection, command.ExpectedVersion); err != nil {
			return err
		}
		if collection.Status != domain.StatusArbitration {
			return domain.NewError(domain.CodeStateConflict, "仅待仲裁批次可预校验")
		}
		if _, err := evidenceOf(state, collectionID, command.EvidenceID); err != nil {
			return err
		}
		decision, err := validateAdjudication(collection, command, latestRevisions(state, collection)[command.EvidenceID], s.now().UTC())
		if err != nil {
			return err
		}
		result = AdjudicationPreview{Valid: true, Message: "决定可提交", EvidenceID: command.EvidenceID, Action: command.Action, FinalSpeciesCode: decision.FinalSpeciesCode, FinalCount: decision.FinalCount, TargetSeatID: decision.TargetSeatID}
		for _, finding := range state.Findings[collectionID] {
			if finding.EvidenceID == command.EvidenceID && finding.Status == domain.FindingOpen {
				result.ClosingFindingIDs = append(result.ClosingFindingIDs, finding.FindingID)
			}
		}
		return nil
	})
	return result, wrapUseCase("仲裁预校验", err)
}

func (s *Service) ApproveReview(collectionID string, command ReviewCommand) (domain.ImageCollection, error) {
	if err := require(command.CommandMeta, RoleExpert); err != nil {
		return domain.ImageCollection{}, err
	}
	var result domain.ImageCollection
	now := s.now().UTC()
	err := s.repo.Update("review.approved", collectionID, now, func(state *persistence.State) error {
		reused, fingerprint, err := reusable(*state, "review:"+collectionID, command.IdempotencyKey, command, &result)
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
		if collection.Status != domain.StatusReview {
			return domain.NewError(domain.CodeStateConflict, "批次尚未进入待复核")
		}
		if evidence.BlockingCount(state.Findings[collectionID]) != 0 {
			return domain.NewError(domain.CodeStateConflict, "仍有阻断发现未关闭")
		}
		from := collection.Status
		collection.ReviewerID = command.Actor
		if err := domain.Transition(&collection, command.ExpectedVersion, domain.StatusFreezable, now); err != nil {
			return err
		}
		state.Collections[collectionID] = collection
		addAuditObject(state, collection, "review.approved", command.Actor, "专家复核通过，批次可冻结", from, now, "collection", collection.CollectionID)
		result = collection
		return remember(state, "review:"+collectionID, command.IdempotencyKey, fingerprint, result)
	})
	return result, wrapUseCase("专家复核", err)
}
