package application

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"wildframe/internal/domain"
	"wildframe/internal/evidence"
	"wildframe/internal/persistence"
)

func (s *Service) GetCollection(collectionID, viewerSeat string) (CollectionView, error) {
	return s.GetCollectionWithQuery(collectionID, CollectionQuery{ViewerSeat: viewerSeat})
}

func (s *Service) GetCollectionWithQuery(collectionID string, query CollectionQuery) (CollectionView, error) {
	var view CollectionView
	err := s.repo.Read(func(state persistence.State) error {
		collection, err := collectionOf(state, collectionID)
		if err != nil {
			return err
		}
		view.Collection = collection
		if query.ViewerSeat != "" && !domain.HasSeat(collection, query.ViewerSeat) {
			return domain.NewError(domain.CodeForbidden, "viewerSeat 不属于当前批次")
		}
		latest := latestRevisions(state, collection)
		items := append([]domain.ImageEvidence(nil), state.Evidence[collectionID]...)
		sort.SliceStable(items, func(i, j int) bool {
			if query.Sort == "capturedAt" && !items[i].CapturedAt.Equal(items[j].CapturedAt) {
				return items[i].CapturedAt.Before(items[j].CapturedAt)
			}
			if items[i].RegisteredAt.Equal(items[j].RegisteredAt) {
				return items[i].EvidenceID < items[j].EvidenceID
			}
			return items[i].RegisteredAt.Before(items[j].RegisteredAt)
		})
		progress := seatProgress(state, collection, query.ViewerSeat, items, latest)
		if query.ViewerSeat != "" {
			view.SeatProgress = &progress
		}
		for _, item := range items {
			if !matchesTaskFilter(state, collection, query, item.EvidenceID, latest) {
				continue
			}
			itemView := EvidenceView{ImageEvidence: item}
			seatMap := latest[item.EvidenceID]
			both := seatMap[collection.AnnotatorSeats[0]].RevisionID != "" && seatMap[collection.AnnotatorSeats[1]].RevisionID != ""
			for _, seat := range collection.AnnotatorSeats {
				revision := seatMap[seat]
				revealed := revision.RevisionID != "" && (both || query.ViewerSeat == seat)
				visibility := SeatVisibility{SeatID: seat, Submitted: revision.RevisionID != "", Revealed: revealed}
				if revealed {
					copy := revision
					visibility.Revision = &copy
				}
				itemView.SeatVisibility = append(itemView.SeatVisibility, visibility)
				if revealed {
					itemView.Annotations = append(itemView.Annotations, revision)
				}
			}
			view.Evidence = append(view.Evidence, itemView)
		}
		for _, finding := range state.Findings[collectionID] {
			if query.FindingRule != "" && finding.RuleCode != query.FindingRule {
				continue
			}
			if query.FindingSeverity != "" && finding.Severity != query.FindingSeverity {
				continue
			}
			if query.FindingStatus != "" && finding.Status != query.FindingStatus {
				continue
			}
			view.Findings = append(view.Findings, finding)
		}
		view.QualitySummary = summarizeQuality(state.Findings[collectionID])
		view.QualityRuns = append([]domain.QualityRun(nil), state.QualityRuns[collectionID]...)
		view.RemediationTasks = append([]domain.RemediationTask(nil), state.RemediationTasks[collectionID]...)
		view.AdjudicationQueue = buildAdjudicationQueue(state, collection, query.QueueFilter)
		for _, decision := range state.Decisions[collectionID] {
			view.Decisions = append(view.Decisions, decision)
		}
		view.Audit = append([]domain.AuditRecord(nil), state.Audits[collectionID]...)
		sort.SliceStable(view.Audit, func(i, j int) bool {
			if view.Audit[i].OccurredAt.Equal(view.Audit[j].OccurredAt) {
				return view.Audit[i].Sequence < view.Audit[j].Sequence
			}
			return view.Audit[i].OccurredAt.Before(view.Audit[j].OccurredAt)
		})
		if err := domain.ValidateAuditTimeline(collectionID, view.Audit); err != nil {
			view.AuditIntegrity = AuditIntegrity{Valid: false, Message: err.Error()}
			view.Audit = nil
		} else {
			view.AuditIntegrity = AuditIntegrity{Valid: true, Message: "审计链连续且时间有序"}
			view.StatusDurations = calculateStatusDurations(collection, state.Audits[collectionID], s.now().UTC())
		}
		if raw := state.Manifests[collectionID]; len(raw) > 0 {
			var manifest evidence.Manifest
			if json.Unmarshal(raw, &manifest) == nil {
				view.Manifest = manifest
			}
		}
		if raw := state.Credentials[collectionID]; len(raw) > 0 {
			var credential evidence.CredentialEnvelope
			if json.Unmarshal(raw, &credential) == nil {
				view.Credential = &credential
			}
		}
		return nil
	})
	return view, err
}

func seatProgress(state persistence.State, collection domain.ImageCollection, seat string, items []domain.ImageEvidence, latest map[string]map[string]domain.AnnotationRevision) SeatProgress {
	result := SeatProgress{SeatID: seat, Total: len(items)}
	if seat == "" {
		return result
	}
	remediation := map[string]bool{}
	for _, task := range state.RemediationTasks[collection.CollectionID] {
		if task.TargetSeatID == seat && task.Status == "open" {
			remediation[task.EvidenceID] = true
		}
	}
	for _, item := range items {
		if latest[item.EvidenceID][seat].RevisionID != "" {
			result.Submitted++
		} else if result.NextEvidenceID == "" {
			result.NextEvidenceID = item.EvidenceID
		}
		if remediation[item.EvidenceID] {
			result.Remediation++
			if result.NextEvidenceID == "" {
				result.NextEvidenceID = item.EvidenceID
			}
		}
	}
	result.Pending = result.Total - result.Submitted
	return result
}

func matchesTaskFilter(state persistence.State, collection domain.ImageCollection, query CollectionQuery, evidenceID string, latest map[string]map[string]domain.AnnotationRevision) bool {
	if query.TaskFilter == "" || query.ViewerSeat == "" {
		return true
	}
	submitted := latest[evidenceID][query.ViewerSeat].RevisionID != ""
	switch query.TaskFilter {
	case "pending":
		return !submitted
	case "submitted":
		return submitted
	case "remediation":
		for _, task := range state.RemediationTasks[collection.CollectionID] {
			if task.EvidenceID == evidenceID && task.TargetSeatID == query.ViewerSeat && task.Status == "open" {
				return true
			}
		}
		return false
	default:
		return true
	}
}

func summarizeQuality(findings []domain.QualityFinding) QualitySummary {
	result := QualitySummary{}
	groups := map[string]*QualityCount{}
	affected := map[string]bool{}
	for _, finding := range findings {
		key := finding.RuleCode + "\x00" + string(finding.Severity) + "\x00" + string(finding.Status)
		if groups[key] == nil {
			groups[key] = &QualityCount{RuleCode: finding.RuleCode, Severity: finding.Severity, Status: finding.Status}
		}
		groups[key].Count++
		if finding.Status == domain.FindingOpen {
			affected[finding.EvidenceID] = true
			if finding.Severity == domain.SeverityBlocking {
				result.OpenBlocking++
			}
			if finding.Severity == domain.SeverityWarning {
				result.OpenWarnings++
			}
		}
	}
	result.AffectedEvidence = len(affected)
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		result.Groups = append(result.Groups, *groups[key])
	}
	return result
}

func buildAdjudicationQueue(state persistence.State, collection domain.ImageCollection, filter string) []AdjudicationQueueItem {
	byEvidence := map[string]*AdjudicationQueueItem{}
	for _, finding := range state.Findings[collection.CollectionID] {
		if finding.Status != domain.FindingOpen || finding.Severity != domain.SeverityBlocking {
			continue
		}
		item := byEvidence[finding.EvidenceID]
		if item == nil {
			item = &AdjudicationQueueItem{EvidenceID: finding.EvidenceID, FirstOpenedAt: finding.OpenedAt, Status: "undecided"}
			byEvidence[finding.EvidenceID] = item
		}
		item.BlockingCount++
		if finding.OpenedAt.Before(item.FirstOpenedAt) {
			item.FirstOpenedAt = finding.OpenedAt
		}
	}
	for _, finding := range state.Findings[collection.CollectionID] {
		if finding.Status == domain.FindingOpen && byEvidence[finding.EvidenceID] != nil {
			byEvidence[finding.EvidenceID].Findings = append(byEvidence[finding.EvidenceID].Findings, finding)
		}
	}
	for evidenceID, decision := range state.Decisions[collection.CollectionID] {
		if decision.DecisionID != "" && byEvidence[evidenceID] == nil {
			byEvidence[evidenceID] = &AdjudicationQueueItem{EvidenceID: evidenceID, FirstOpenedAt: decision.DecidedAt, Status: "decided"}
		}
	}
	latest := latestRevisions(state, collection)
	result := make([]AdjudicationQueueItem, 0, len(byEvidence))
	for evidenceID, item := range byEvidence {
		seatMap := latest[evidenceID]
		if seatMap[collection.AnnotatorSeats[0]].RevisionID != "" && seatMap[collection.AnnotatorSeats[1]].RevisionID != "" {
			item.Revisions = []domain.AnnotationRevision{seatMap[collection.AnnotatorSeats[0]], seatMap[collection.AnnotatorSeats[1]]}
		}
		if decision := state.Decisions[collection.CollectionID][evidenceID]; decision.DecisionID != "" {
			copy := decision
			item.Decision = &copy
			item.Status = "decided"
		}
		for index := len(state.RemediationTasks[collection.CollectionID]) - 1; index >= 0; index-- {
			task := state.RemediationTasks[collection.CollectionID][index]
			if task.EvidenceID == evidenceID {
				copy := task
				item.RemediationTask = &copy
				if task.Status == "open" {
					item.Status = "remediation"
				} else if task.Status == "returned" && item.BlockingCount > 0 {
					item.Status = "undecided"
				}
				break
			}
		}
		if filter == "undecided" && item.Status != "undecided" || filter == "decided" && item.Status != "decided" || filter == "remediation" && item.Status != "remediation" {
			continue
		}
		result = append(result, *item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].BlockingCount != result[j].BlockingCount {
			return result[i].BlockingCount > result[j].BlockingCount
		}
		if !result[i].FirstOpenedAt.Equal(result[j].FirstOpenedAt) {
			return result[i].FirstOpenedAt.Before(result[j].FirstOpenedAt)
		}
		return result[i].EvidenceID < result[j].EvidenceID
	})
	return result
}

func calculateStatusDurations(collection domain.ImageCollection, records []domain.AuditRecord, now time.Time) []StatusDuration {
	type acc struct {
		first        time.Time
		last         *time.Time
		seconds      int64
		currentStart time.Time
	}
	values := map[domain.CollectionStatus]*acc{}
	if len(records) == 0 {
		return nil
	}
	current := records[0].FromStatus
	entered := collection.CreatedAt
	if entered.IsZero() {
		entered = records[0].OccurredAt
	}
	for _, record := range records {
		if record.ToStatus == current {
			continue
		}
		a := values[current]
		if a == nil {
			a = &acc{first: entered}
			values[current] = a
		}
		a.seconds += int64(record.OccurredAt.Sub(entered).Seconds())
		left := record.OccurredAt
		a.last = &left
		current = record.ToStatus
		entered = record.OccurredAt
	}
	a := values[current]
	if a == nil {
		a = &acc{first: entered}
		values[current] = a
	}
	a.seconds += int64(now.Sub(entered).Seconds())
	a.currentStart = entered
	statuses := make([]string, 0, len(values))
	for status := range values {
		statuses = append(statuses, string(status))
	}
	sort.Strings(statuses)
	result := make([]StatusDuration, 0, len(statuses))
	for _, raw := range statuses {
		status := domain.CollectionStatus(raw)
		v := values[status]
		result = append(result, StatusDuration{Status: status, FirstEnteredAt: v.first, LastLeftAt: v.last, DurationSeconds: v.seconds, Current: status == current})
	}
	return result
}

func (s *Service) QueryAudit(collectionID string, query AuditQuery) (AuditPage, error) {
	var page AuditPage
	if query.Limit == 0 {
		query.Limit = 50
	}
	if query.Limit < 1 || query.Limit > 200 {
		return page, domain.NewError(domain.CodeValidation, "limit 必须在 1 到 200 之间")
	}
	if query.From != nil && query.To != nil && query.To.Before(*query.From) {
		return page, domain.NewError(domain.CodeValidation, "审计起止时间范围无效")
	}
	err := s.repo.Read(func(state persistence.State) error {
		collection, err := collectionOf(state, collectionID)
		if err != nil {
			return err
		}
		records := state.Audits[collectionID]
		if query.After > uint64(len(records)) {
			return domain.NewError(domain.CodeValidation, "after 游标超出审计链范围")
		}
		if err := domain.ValidateAuditTimeline(collectionID, records); err != nil {
			page.Integrity = AuditIntegrity{Valid: false, Message: err.Error()}
			return nil
		}
		page.Integrity = AuditIntegrity{Valid: true, Message: "审计链连续且时间有序"}
		page.StatusDurations = calculateStatusDurations(collection, records, s.now().UTC())
		for _, record := range records {
			if record.Sequence <= query.After {
				continue
			}
			if query.Actor != "" && record.Actor != query.Actor || query.Action != "" && !strings.Contains(record.Action, query.Action) || query.From != nil && record.OccurredAt.Before(*query.From) || query.To != nil && record.OccurredAt.After(*query.To) || query.FromStatus != "" && record.FromStatus != query.FromStatus || query.ToStatus != "" && record.ToStatus != query.ToStatus {
				continue
			}
			if len(page.Records) == query.Limit {
				page.NextCursor = page.Records[len(page.Records)-1].Sequence
				break
			}
			page.Records = append(page.Records, record)
		}
		return nil
	})
	return page, err
}

func (s *Service) ListCollections() ([]domain.ImageCollection, error) {
	collections := make([]domain.ImageCollection, 0)
	err := s.repo.Read(func(state persistence.State) error {
		for _, collection := range state.Collections {
			collections = append(collections, collection)
		}
		sort.Slice(collections, func(i, j int) bool { return collections[i].UpdatedAt.After(collections[j].UpdatedAt) })
		return nil
	})
	return collections, err
}

func (s *Service) OpenBlob(key string) (interface {
	Read([]byte) (int, error)
	Close() error
}, error) {
	return s.blobs.Open(key)
}

func (s *Service) Healthy() bool { return s.repo.Recovered() }

func (s *Service) RecoveryStatus() persistence.RecoveryStatus { return s.repo.RecoveryStatus() }
