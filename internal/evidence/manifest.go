package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"sort"
	"time"

	"wildframe/internal/domain"
)

type Manifest struct {
	SchemaVersion  int             `json:"schemaVersion"`
	CollectionID   string          `json:"collectionID"`
	ReserveName    string          `json:"reserveName"`
	CameraSite     string          `json:"cameraSite"`
	CapturedFrom   string          `json:"capturedFrom"`
	CapturedTo     string          `json:"capturedTo"`
	RuleSetVersion string          `json:"ruleSetVersion"`
	Entries        []ManifestEntry `json:"entries"`
}

type ManifestEntry struct {
	EvidenceID       string `json:"evidenceID"`
	OriginalName     string `json:"originalName"`
	CapturedAt       string `json:"capturedAt"`
	SHA256Digest     string `json:"sha256Digest"`
	MediaType        string `json:"mediaType"`
	ByteSize         int64  `json:"byteSize"`
	SpeciesCode      string `json:"speciesCode"`
	IndividualCount  int    `json:"individualCount"`
	ConclusionSource string `json:"conclusionSource"`
}

type ManifestStatistics struct {
	EntryCount        int            `json:"entryCount"`
	TotalBytes        int64          `json:"totalBytes"`
	Species           map[string]int `json:"species"`
	ConclusionSources map[string]int `json:"conclusionSources"`
}

type FieldChange struct {
	Field  string `json:"field"`
	Before any    `json:"before,omitempty"`
	After  any    `json:"after,omitempty"`
}
type ManifestDifference struct {
	EvidenceID string        `json:"evidenceID"`
	Kind       string        `json:"kind"`
	Changes    []FieldChange `json:"changes,omitempty"`
}

func SummarizeManifest(manifest Manifest) ManifestStatistics {
	result := ManifestStatistics{EntryCount: len(manifest.Entries), Species: map[string]int{}, ConclusionSources: map[string]int{}}
	for _, entry := range manifest.Entries {
		result.TotalBytes += entry.ByteSize
		result.Species[entry.SpeciesCode]++
		result.ConclusionSources[entry.ConclusionSource]++
	}
	return result
}

func DiffManifests(previous, current Manifest) []ManifestDifference {
	before, after := map[string]ManifestEntry{}, map[string]ManifestEntry{}
	for _, entry := range previous.Entries {
		before[entry.EvidenceID] = entry
	}
	for _, entry := range current.Entries {
		after[entry.EvidenceID] = entry
	}
	ids := make([]string, 0, len(before)+len(after))
	seen := map[string]bool{}
	for id := range before {
		ids = append(ids, id)
		seen[id] = true
	}
	for id := range after {
		if !seen[id] {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	result := make([]ManifestDifference, 0)
	for _, id := range ids {
		old, oldOK := before[id]
		next, nextOK := after[id]
		if !oldOK {
			result = append(result, ManifestDifference{EvidenceID: id, Kind: "added"})
			continue
		}
		if !nextOK {
			result = append(result, ManifestDifference{EvidenceID: id, Kind: "removed"})
			continue
		}
		changes := entryChanges(old, next)
		if len(changes) > 0 {
			result = append(result, ManifestDifference{EvidenceID: id, Kind: "changed", Changes: changes})
		}
	}
	return result
}

func entryChanges(before, after ManifestEntry) []FieldChange {
	left, right := reflect.ValueOf(before), reflect.ValueOf(after)
	typeOf := left.Type()
	changes := make([]FieldChange, 0)
	for i := 1; i < left.NumField(); i++ {
		if !reflect.DeepEqual(left.Field(i).Interface(), right.Field(i).Interface()) {
			name := typeOf.Field(i).Tag.Get("json")
			changes = append(changes, FieldChange{Field: name, Before: left.Field(i).Interface(), After: right.Field(i).Interface()})
		}
	}
	return changes
}

func BuildManifest(collection domain.ImageCollection, items []domain.ImageEvidence, annotations map[string]map[string]domain.AnnotationRevision, decisions map[string]domain.AdjudicationDecision) (Manifest, []byte, string, error) {
	manifest := Manifest{
		SchemaVersion: 1, CollectionID: collection.CollectionID, ReserveName: collection.ReserveName,
		CameraSite: collection.CameraSite, CapturedFrom: collection.CapturedFrom.UTC().Format(time.RFC3339Nano),
		CapturedTo: collection.CapturedTo.UTC().Format(time.RFC3339Nano), RuleSetVersion: collection.RuleSetVersion,
		Entries: make([]ManifestEntry, 0, len(items)),
	}
	sorted := append([]domain.ImageEvidence(nil), items...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].EvidenceID < sorted[j].EvidenceID })
	for _, item := range sorted {
		species, count, source, err := conclusion(collection, item.EvidenceID, annotations[item.EvidenceID], decisions[item.EvidenceID])
		if err != nil {
			return Manifest{}, nil, "", err
		}
		manifest.Entries = append(manifest.Entries, ManifestEntry{
			EvidenceID: item.EvidenceID, OriginalName: item.OriginalName,
			CapturedAt: item.CapturedAt.UTC().Format(time.RFC3339Nano), SHA256Digest: item.SHA256Digest,
			MediaType: item.MediaType, ByteSize: item.ByteSize, SpeciesCode: species,
			IndividualCount: count, ConclusionSource: source,
		})
	}
	canonical, err := json.Marshal(manifest)
	if err != nil {
		return Manifest{}, nil, "", err
	}
	digest := sha256.Sum256(canonical)
	return manifest, canonical, hex.EncodeToString(digest[:]), nil
}

func conclusion(collection domain.ImageCollection, evidenceID string, seats map[string]domain.AnnotationRevision, decision domain.AdjudicationDecision) (string, int, string, error) {
	if decision.DecisionID != "" && decision.Action != "request_remediation" {
		return decision.FinalSpeciesCode, decision.FinalCount, "adjudication", nil
	}
	a, aOK := seats[collection.AnnotatorSeats[0]]
	b, bOK := seats[collection.AnnotatorSeats[1]]
	if !aOK || !bOK || a.SpeciesCode != b.SpeciesCode || a.IndividualCount != b.IndividualCount {
		return "", 0, "", domain.NewError(domain.CodeStateConflict, "证据 %s 尚无可冻结结论", evidenceID)
	}
	return a.SpeciesCode, a.IndividualCount, "seat_consensus", nil
}
