package domain

import "time"

func NewCollection(id string, input CreateCollectionInput, now time.Time) (ImageCollection, error) {
	if err := ValidateCollection(input); err != nil {
		return ImageCollection{}, err
	}
	return ImageCollection{
		CollectionID: id, ReserveName: input.ReserveName, CameraSite: input.CameraSite,
		CapturedFrom: input.CapturedFrom.UTC(), CapturedTo: input.CapturedTo.UTC(),
		RuleSetVersion: input.RuleSetVersion, AnnotatorSeats: [2]string{input.SeatA, input.SeatB},
		Status: StatusDraft, Version: 1, CreatedAt: now.UTC(), UpdatedAt: now.UTC(),
	}, nil
}

func CheckVersion(collection ImageCollection, expected int64) error {
	if expected != collection.Version {
		return NewError(CodeVersionConflict, "版本冲突：期望 %d，当前 %d", expected, collection.Version)
	}
	return nil
}

func Touch(collection *ImageCollection, now time.Time) {
	collection.Version++
	collection.UpdatedAt = now.UTC()
}

func Transition(collection *ImageCollection, expected int64, next CollectionStatus, now time.Time) error {
	if err := CheckVersion(*collection, expected); err != nil {
		return err
	}
	if collection.Status == StatusFrozen && next != StatusReleased {
		return NewError(CodeStateConflict, "已冻结批次只能进入已发布")
	}
	if collection.Status == StatusReleased {
		return NewError(CodeStateConflict, "已发布批次不能变更状态")
	}
	if !validTransition(collection.Status, next) {
		return NewError(CodeStateConflict, "不允许从 %s 进入 %s", collection.Status, next)
	}
	collection.Status = next
	Touch(collection, now)
	return nil
}

func validTransition(from, to CollectionStatus) bool {
	allowed := map[CollectionStatus]map[CollectionStatus]bool{
		StatusDraft:       {StatusAnnotating: true},
		StatusAnnotating:  {StatusReview: true, StatusArbitration: true},
		StatusArbitration: {StatusRemediation: true, StatusReview: true},
		StatusRemediation: {StatusArbitration: true, StatusReview: true},
		StatusReview:      {StatusArbitration: true, StatusFreezable: true},
		StatusFreezable:   {StatusFrozen: true},
		StatusFrozen:      {StatusReleased: true},
	}
	return allowed[from][to]
}

func EnsureMutable(collection ImageCollection) error {
	if !collection.Status.Mutable() {
		return NewError(CodeStateConflict, "冻结后禁止修改证据、标注和裁决")
	}
	return nil
}

func HasSeat(collection ImageCollection, seat string) bool {
	return collection.AnnotatorSeats[0] == seat || collection.AnnotatorSeats[1] == seat
}
