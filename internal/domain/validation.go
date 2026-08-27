package domain

import (
	"regexp"
	"strings"
	"time"
)

var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type CreateCollectionInput struct {
	ReserveName    string
	CameraSite     string
	CapturedFrom   time.Time
	CapturedTo     time.Time
	RuleSetVersion string
	SeatA          string
	SeatB          string
}

func ValidateCollection(input CreateCollectionInput) error {
	if strings.TrimSpace(input.ReserveName) == "" {
		return NewError(CodeValidation, "保护区不能为空")
	}
	if strings.TrimSpace(input.CameraSite) == "" {
		return NewError(CodeValidation, "相机点位不能为空")
	}
	if input.CapturedFrom.IsZero() || input.CapturedTo.IsZero() || input.CapturedTo.Before(input.CapturedFrom) {
		return NewError(CodeValidation, "拍摄时段无效")
	}
	if strings.TrimSpace(input.RuleSetVersion) == "" {
		return NewError(CodeValidation, "标注规则版本不能为空")
	}
	if strings.TrimSpace(input.SeatA) == "" || strings.TrimSpace(input.SeatB) == "" || input.SeatA == input.SeatB {
		return NewError(CodeValidation, "必须配置两个不同的标注席位")
	}
	return nil
}

func ValidateEvidence(collection ImageCollection, item ImageEvidence) error {
	if !collection.Status.Mutable() {
		return NewError(CodeStateConflict, "批次冻结后不能登记证据")
	}
	if collection.Status != StatusAnnotating {
		return NewError(CodeStateConflict, "只有标注中批次可以登记新证据")
	}
	if strings.TrimSpace(item.OriginalName) == "" || strings.ContainsAny(item.OriginalName, `/\\`) {
		return NewError(CodeValidation, "原始文件名无效")
	}
	if item.CameraSite != collection.CameraSite {
		return NewError(CodeValidation, "证据相机点位超出批次边界")
	}
	if item.CapturedAt.Before(collection.CapturedFrom) || item.CapturedAt.After(collection.CapturedTo) {
		return NewError(CodeValidation, "证据采集时间超出批次边界")
	}
	if !digestPattern.MatchString(item.SHA256Digest) {
		return NewError(CodeValidation, "SHA-256 摘要格式无效")
	}
	if item.ByteSize <= 0 || item.PixelWidth <= 0 || item.PixelHeight <= 0 {
		return NewError(CodeValidation, "媒体大小和像素尺寸必须为正数")
	}
	if item.MediaType != "image/jpeg" && item.MediaType != "image/png" && item.MediaType != "image/webp" {
		return NewError(CodeValidation, "仅支持 JPEG、PNG 或 WebP 影像")
	}
	return nil
}

func ValidateAnnotation(collection ImageCollection, item AnnotationRevision) error {
	if !collection.Status.Mutable() || !collection.Status.CanAnnotate() {
		return NewError(CodeStateConflict, "当前状态不能提交标注")
	}
	if item.SeatID != collection.AnnotatorSeats[0] && item.SeatID != collection.AnnotatorSeats[1] {
		return NewError(CodeForbidden, "提交人不是批次标注席位")
	}
	if item.Identifiability != "identifiable" && item.Identifiability != "unidentifiable" {
		return NewError(CodeValidation, "可辨识性取值无效")
	}
	if item.Identifiability == "identifiable" && strings.TrimSpace(item.SpeciesCode) == "" {
		return NewError(CodeValidation, "可辨识影像必须填写物种")
	}
	if item.Identifiability == "unidentifiable" && strings.TrimSpace(item.UnidentifiableReason) == "" {
		return NewError(CodeValidation, "无法辨识时必须填写原因")
	}
	if item.IndividualCount < 0 || item.IndividualCount > 999 {
		return NewError(CodeValidation, "个体数量必须在 0 到 999 之间")
	}
	if item.Confidence < 0 || item.Confidence > 1 {
		return NewError(CodeValidation, "置信度必须在 0 到 1 之间")
	}
	return nil
}
