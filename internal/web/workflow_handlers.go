package web

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"wildframe/internal/application"
	"wildframe/internal/domain"
	"wildframe/internal/evidence"
)

func (s *Server) HandleSubmitAnnotation(writer http.ResponseWriter, request *http.Request) {
	var command application.SubmitAnnotationCommand
	if err := decodeJSON(writer, request, &command); err != nil {
		writeError(writer, err)
		return
	}
	revision, err := s.service.SubmitAnnotation(request.PathValue("collectionID"), command)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusCreated, revision)
}

func (s *Server) HandlePreviewAdjudication(writer http.ResponseWriter, request *http.Request) {
	var command application.AdjudicateCommand
	if err := decodeJSON(writer, request, &command); err != nil {
		writeError(writer, err)
		return
	}
	preview, err := s.service.PreviewAdjudication(request.PathValue("collectionID"), command)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, preview)
}

func (s *Server) HandleRecalculateQuality(writer http.ResponseWriter, request *http.Request) {
	var command application.RecalculateQualityCommand
	if err := decodeJSON(writer, request, &command); err != nil {
		writeError(writer, err)
		return
	}
	run, err := s.service.RecalculateQuality(request.PathValue("collectionID"), command)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, run)
}

func (s *Server) HandleAuditQuery(writer http.ResponseWriter, request *http.Request) {
	values := request.URL.Query()
	query := application.AuditQuery{Actor: strings.TrimSpace(values.Get("actor")), Action: strings.TrimSpace(values.Get("action")), FromStatus: domain.CollectionStatus(values.Get("fromStatus")), ToStatus: domain.CollectionStatus(values.Get("toStatus"))}
	if raw := values.Get("after"); raw != "" {
		value, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			writeError(writer, domain.NewError(domain.CodeValidation, "after 游标必须为非负整数"))
			return
		}
		query.After = value
	}
	if raw := values.Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			writeError(writer, domain.NewError(domain.CodeValidation, "limit 必须为整数"))
			return
		}
		query.Limit = value
	}
	parseTime := func(name string) (*time.Time, error) {
		raw := values.Get(name)
		if raw == "" {
			return nil, nil
		}
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return nil, domain.NewError(domain.CodeValidation, "%s 必须为 RFC3339 时间", name)
		}
		return &value, nil
	}
	var err error
	if query.From, err = parseTime("from"); err != nil {
		writeError(writer, err)
		return
	}
	if query.To, err = parseTime("to"); err != nil {
		writeError(writer, err)
		return
	}
	page, err := s.service.QueryAudit(request.PathValue("collectionID"), query)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, page)
}

func (s *Server) HandleAdjudicate(writer http.ResponseWriter, request *http.Request) {
	var command application.AdjudicateCommand
	if err := decodeJSON(writer, request, &command); err != nil {
		writeError(writer, err)
		return
	}
	decision, err := s.service.Adjudicate(request.PathValue("collectionID"), command)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusCreated, decision)
}

func (s *Server) HandleReview(writer http.ResponseWriter, request *http.Request) {
	var command application.ReviewCommand
	if err := decodeJSON(writer, request, &command); err != nil {
		writeError(writer, err)
		return
	}
	collection, err := s.service.ApproveReview(request.PathValue("collectionID"), command)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, collection)
}

func (s *Server) HandleManifestPreview(writer http.ResponseWriter, request *http.Request) {
	preview, err := s.service.PreviewManifest(request.PathValue("collectionID"))
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, preview)
}

func (s *Server) HandleFreeze(writer http.ResponseWriter, request *http.Request) {
	var command application.FreezeCommand
	if err := decodeJSON(writer, request, &command); err != nil {
		writeError(writer, err)
		return
	}
	collection, err := s.service.Freeze(request.PathValue("collectionID"), command)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, collection)
}

func (s *Server) HandleIssueCredential(writer http.ResponseWriter, request *http.Request) {
	var command application.IssueCommand
	if err := decodeJSON(writer, request, &command); err != nil {
		writeError(writer, err)
		return
	}
	credential, err := s.service.IssueCredential(request.PathValue("collectionID"), command)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusCreated, credential)
}

func (s *Server) HandleVerifyCredential(writer http.ResponseWriter, request *http.Request) {
	var credential evidence.CredentialEnvelope
	if mediaType := request.Header.Get("Content-Type"); !strings.HasPrefix(mediaType, "application/json") {
		writeError(writer, domain.NewError(domain.CodeValidation, "Content-Type 必须为 application/json"))
		return
	}
	request.Body = http.MaxBytesReader(writer, request.Body, 256<<10)
	raw, err := io.ReadAll(request.Body)
	if err != nil || len(raw) == 0 {
		writeError(writer, domain.NewError(domain.CodeValidation, "凭据 JSON 为空或超过大小限制"))
		return
	}
	if err := json.Unmarshal(raw, &credential); err != nil {
		writeError(writer, domain.NewError(domain.CodeValidation, "凭据 JSON 格式无效"))
		return
	}
	result := s.service.VerifyCredential(credential)
	writeJSON(writer, http.StatusOK, result)
}
