package web

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"wildframe/internal/application"
	"wildframe/internal/domain"
	"wildframe/internal/persistence"
)

func (s *Server) HandleListCollections(writer http.ResponseWriter, _ *http.Request) {
	collections, err := s.service.ListCollections()
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"collections": collections})
}

func (s *Server) HandleCreateCollection(writer http.ResponseWriter, request *http.Request) {
	var command application.CreateCollectionCommand
	if err := decodeJSON(writer, request, &command); err != nil {
		writeError(writer, err)
		return
	}
	collection, err := s.service.CreateCollection(command)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusCreated, collection)
}

func (s *Server) HandleGetCollection(writer http.ResponseWriter, request *http.Request) {
	query := application.CollectionQuery{ViewerSeat: clientSeat(request), TaskFilter: request.URL.Query().Get("task"), Sort: request.URL.Query().Get("sort"), FindingRule: request.URL.Query().Get("rule"), FindingSeverity: domain.Severity(request.URL.Query().Get("severity")), FindingStatus: domain.FindingStatus(request.URL.Query().Get("findingStatus")), QueueFilter: request.URL.Query().Get("queue")}
	if query.TaskFilter != "" && query.TaskFilter != "pending" && query.TaskFilter != "submitted" && query.TaskFilter != "remediation" {
		writeError(writer, domain.NewError(domain.CodeValidation, "task 筛选值无效"))
		return
	}
	if query.Sort != "" && query.Sort != "registered" && query.Sort != "capturedAt" {
		writeError(writer, domain.NewError(domain.CodeValidation, "sort 排序值无效"))
		return
	}
	view, err := s.service.GetCollectionWithQuery(request.PathValue("collectionID"), query)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, view)
}

const maxBatchRequestBytes int64 = 96 << 20

func (s *Server) HandleRegisterEvidenceBatch(writer http.ResponseWriter, request *http.Request) {
	request.Body = http.MaxBytesReader(writer, request.Body, maxBatchRequestBytes)
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" {
		writeError(writer, domain.NewError(domain.CodeValidation, "Content-Type 必须为 multipart/form-data"))
		return
	}
	if err := request.ParseMultipartForm(2 << 20); err != nil {
		writeError(writer, domain.NewError(domain.CodeValidation, "批量上传表单无效：%s", err))
		return
	}
	defer request.MultipartForm.RemoveAll()
	headers := append([]*multipart.FileHeader(nil), request.MultipartForm.File["files"]...)
	headers = append(headers, request.MultipartForm.File["file"]...)
	if len(headers) == 0 || len(headers) > application.MaxBatchItems {
		writeError(writer, domain.NewError(domain.CodeValidation, "批量影像数量必须在 1 到 %d 之间", application.MaxBatchItems))
		return
	}
	expectedVersion, err := strconv.ParseInt(request.FormValue("expectedVersion"), 10, 64)
	if err != nil {
		writeError(writer, domain.NewError(domain.CodeValidation, "expectedVersion 必须为整数"))
		return
	}
	values := request.MultipartForm.Value
	valueAt := func(name string, index int) string {
		items := values[name]
		if index < len(items) {
			return items[index]
		}
		if len(items) == 1 {
			return items[0]
		}
		return ""
	}
	items := make([]application.BatchEvidenceItem, 0, len(headers))
	for index, header := range headers {
		file, openErr := header.Open()
		if openErr != nil {
			writeError(writer, domain.NewError(domain.CodeValidation, "无法读取第 %d 个文件", index+1))
			return
		}
		payload, readErr := io.ReadAll(io.LimitReader(file, persistence.MaxBlobBytes+1))
		_ = file.Close()
		if readErr != nil {
			writeError(writer, domain.NewError(domain.CodeValidation, "无法读取第 %d 个文件", index+1))
			return
		}
		capturedAt, parseErr := time.Parse(time.RFC3339, valueAt("capturedAt", index))
		if parseErr != nil {
			capturedAt = time.Time{}
		}
		width, _ := strconv.Atoi(valueAt("pixelWidth", index))
		height, _ := strconv.Atoi(valueAt("pixelHeight", index))
		clientID := valueAt("clientItemID", index)
		if clientID == "" {
			clientID = strconv.Itoa(index + 1)
		}
		items = append(items, application.BatchEvidenceItem{ClientItemID: clientID, OriginalName: header.Filename, CapturedAt: capturedAt, CameraSite: valueAt("cameraSite", index), PixelWidth: width, PixelHeight: height, Payload: payload})
	}
	command := application.RegisterEvidenceBatchCommand{CommandMeta: application.CommandMeta{Actor: request.FormValue("actor"), Role: application.Role(request.FormValue("role")), ExpectedVersion: expectedVersion, IdempotencyKey: request.FormValue("idempotencyKey")}, Items: items}
	result, err := s.service.RegisterEvidenceBatch(request.PathValue("collectionID"), command)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (s *Server) HandleRegisterEvidence(writer http.ResponseWriter, request *http.Request) {
	request.Body = http.MaxBytesReader(writer, request.Body, persistence.MaxBlobBytes+(1<<20))
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" {
		writeError(writer, domain.NewError(domain.CodeValidation, "Content-Type 必须为 multipart/form-data"))
		return
	}
	if err := request.ParseMultipartForm(persistence.MaxBlobBytes); err != nil {
		writeError(writer, domain.NewError(domain.CodeValidation, "上传表单无效：%s", err))
		return
	}
	file, header, err := request.FormFile("file")
	if err != nil {
		writeError(writer, domain.NewError(domain.CodeValidation, "缺少 file 影像载荷"))
		return
	}
	defer file.Close()
	payload, err := io.ReadAll(io.LimitReader(file, persistence.MaxBlobBytes+1))
	if err != nil || int64(len(payload)) > persistence.MaxBlobBytes {
		writeError(writer, domain.NewError(domain.CodeValidation, "读取影像失败或超过大小限制"))
		return
	}
	digestBytes := sha256.Sum256(payload)
	actualDigest := hex.EncodeToString(digestBytes[:])
	declaredDigest := strings.ToLower(strings.TrimSpace(request.FormValue("sha256Digest")))
	if declaredDigest != "" && declaredDigest != actualDigest {
		writeError(writer, domain.NewError(domain.CodeValidation, "浏览器摘要与上传载荷不一致"))
		return
	}
	capturedAt, err := time.Parse(time.RFC3339, request.FormValue("capturedAt"))
	if err != nil {
		writeError(writer, domain.NewError(domain.CodeValidation, "capturedAt 必须为 RFC3339 时间"))
		return
	}
	width, errW := strconv.Atoi(request.FormValue("pixelWidth"))
	height, errH := strconv.Atoi(request.FormValue("pixelHeight"))
	if errW != nil || errH != nil {
		writeError(writer, domain.NewError(domain.CodeValidation, "像素尺寸必须为整数"))
		return
	}
	expectedVersion, err := strconv.ParseInt(request.FormValue("expectedVersion"), 10, 64)
	if err != nil {
		writeError(writer, domain.NewError(domain.CodeValidation, "expectedVersion 必须为整数"))
		return
	}
	detectedType := http.DetectContentType(payload)
	command := application.RegisterEvidenceCommand{
		CommandMeta:  application.CommandMeta{Actor: request.FormValue("actor"), Role: application.Role(request.FormValue("role")), ExpectedVersion: expectedVersion, IdempotencyKey: request.FormValue("idempotencyKey")},
		OriginalName: header.Filename, CapturedAt: capturedAt, CameraSite: request.FormValue("cameraSite"), SHA256Digest: actualDigest,
		MediaType: detectedType, PixelWidth: width, PixelHeight: height, Payload: payload,
	}
	item, err := s.service.RegisterEvidence(request.PathValue("collectionID"), command)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusCreated, item)
}

func (s *Server) HandleBlob(writer http.ResponseWriter, request *http.Request) {
	view, err := s.service.GetCollection(request.PathValue("collectionID"), "")
	if err != nil {
		writeError(writer, err)
		return
	}
	key := request.PathValue("blobKey")
	allowed := false
	for _, item := range view.Evidence {
		if item.BlobKey == key {
			allowed = true
			writer.Header().Set("Content-Type", item.MediaType)
			break
		}
	}
	if !allowed {
		writeError(writer, domain.NewError(domain.CodeNotFound, "载荷不存在"))
		return
	}
	file, err := s.service.OpenBlob(key)
	if err != nil {
		writeError(writer, domain.NewError(domain.CodeNotFound, "载荷不存在"))
		return
	}
	defer file.Close()
	writer.Header().Set("Cache-Control", "private, max-age=60")
	_, _ = io.Copy(writer, file)
}
