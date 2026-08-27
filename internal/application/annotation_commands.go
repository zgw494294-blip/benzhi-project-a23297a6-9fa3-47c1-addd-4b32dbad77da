package application

import (
	"fmt"
	"time"

	"wildframe/internal/domain"
	"wildframe/internal/evidence"
	"wildframe/internal/persistence"
)

func (s *Service) SubmitAnnotation(collectionID string, command SubmitAnnotationCommand) (domain.AnnotationRevision, error) {
	if err := require(command.CommandMeta, RoleAnnotator); err != nil {
		return domain.AnnotationRevision{}, err
	}
	if command.Actor != command.SeatID {
		return domain.AnnotationRevision{}, domain.NewError(domain.CodeForbidden, "标注席位只能以自身身份提交")
	}
	var result domain.AnnotationRevision
	now := s.now().UTC()
	scope := "annotate:" + collectionID + ":" + command.EvidenceID + ":" + command.SeatID
	err := s.repo.Update("annotation.submitted", collectionID, now, func(state *persistence.State) error {
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
		item, err := evidenceOf(*state, collectionID, command.EvidenceID)
		if err != nil {
			return err
		}
		if collection.Status == domain.StatusRemediation {
			task, ok := openRemediationTask(*state, collectionID)
			if !ok || task.EvidenceID != command.EvidenceID || task.TargetSeatID != command.SeatID {
				return domain.NewError(domain.CodeForbidden, "整改仅允许被专家指定的证据和席位提交")
			}
			if command.SupersedesRevisionID != task.BaselineRevisionID {
				return domain.NewError(domain.CodeVersionConflict, "整改修订必须 supersede 指定基线 %s", task.BaselineRevisionID)
			}
		}
		latest := latestRevisions(*state, collection)
		previous := latest[item.EvidenceID][command.SeatID]
		revision := domain.AnnotationRevision{
			RevisionID: newID("rev"), EvidenceID: item.EvidenceID, SeatID: command.SeatID,
			SpeciesCode: command.SpeciesCode, IndividualCount: command.IndividualCount, Confidence: command.Confidence,
			Identifiability: command.Identifiability, UnidentifiableReason: command.UnidentifiableReason,
			Note: command.Note, SupersedesRevisionID: previous.RevisionID, SubmittedAt: now,
		}
		if err := domain.ValidateAnnotation(collection, revision); err != nil {
			return err
		}
		from := collection.Status
		state.Revisions[collectionID] = append(state.Revisions[collectionID], revision)
		latest = latestRevisions(*state, collection)
		recalculated := s.quality.EvaluateEvidence(collection, item, latest[item.EvidenceID])
		state.Findings[collectionID] = evidence.MergeAffected(state.Findings[collectionID], item.EvidenceID, recalculated, now)
		if collection.Status == domain.StatusRemediation {
			for index := range state.RemediationTasks[collectionID] {
				task := &state.RemediationTasks[collectionID][index]
				if task.Status == "open" && task.EvidenceID == item.EvidenceID && task.TargetSeatID == command.SeatID {
					returned := now
					task.Status = "returned"
					task.ResultRevisionID = revision.RevisionID
					task.ReturnedAt = &returned
					break
				}
			}
			if evidence.BlockingCount(state.Findings[collectionID]) > 0 {
				collection.Status = domain.StatusArbitration
			} else if !hasOpenRemediationTasks(*state, collectionID) {
				collection.Status = domain.StatusReview
			}
		}
		if allSeatsComplete(*state, collection, latest) {
			if evidence.BlockingCount(state.Findings[collectionID]) > 0 {
				collection.Status = domain.StatusArbitration
			} else {
				collection.Status = domain.StatusReview
			}
		}
		domain.Touch(&collection, now)
		state.Collections[collectionID] = collection
		detail := fmt.Sprintf("席位 %s 提交证据 %s 的第 %d 次修订", command.SeatID, item.EvidenceID, revisionNumber(state.Revisions[collectionID], item.EvidenceID, command.SeatID))
		addAuditObject(state, collection, "annotation.submitted", command.Actor, detail, from, now, "revision", revision.RevisionID)
		result = revision
		return remember(state, scope, command.IdempotencyKey, fingerprint, result)
	})
	return result, err
}

func openRemediationTask(state persistence.State, collectionID string) (domain.RemediationTask, bool) {
	for _, task := range state.RemediationTasks[collectionID] {
		if task.Status == "open" {
			return task, true
		}
	}
	return domain.RemediationTask{}, false
}
func hasOpenRemediationTasks(state persistence.State, collectionID string) bool {
	_, ok := openRemediationTask(state, collectionID)
	return ok
}

func allSeatsComplete(state persistence.State, collection domain.ImageCollection, latest map[string]map[string]domain.AnnotationRevision) bool {
	items := state.Evidence[collection.CollectionID]
	if len(items) == 0 {
		return false
	}
	for _, item := range items {
		if latest[item.EvidenceID][collection.AnnotatorSeats[0]].RevisionID == "" || latest[item.EvidenceID][collection.AnnotatorSeats[1]].RevisionID == "" {
			return false
		}
	}
	return true
}

func revisionNumber(revisions []domain.AnnotationRevision, evidenceID, seatID string) int {
	count := 0
	for _, revision := range revisions {
		if revision.EvidenceID == evidenceID && revision.SeatID == seatID {
			count++
		}
	}
	return count
}

func resolveEvidenceFindings(findings []domain.QualityFinding, evidenceID string, now time.Time) []domain.QualityFinding {
	for index := range findings {
		if findings[index].EvidenceID == evidenceID && findings[index].Status == domain.FindingOpen {
			findings[index].Status = domain.FindingResolved
			resolved := now.UTC()
			findings[index].ResolvedAt = &resolved
		}
	}
	return findings
}
