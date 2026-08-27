package web

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	"wildframe/internal/domain"
)

const maxJSONBytes int64 = 1 << 20

type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeError(writer http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	code := domain.ErrorCodeOf(err)
	switch code {
	case domain.CodeValidation:
		status = http.StatusBadRequest
	case domain.CodeNotFound:
		status = http.StatusNotFound
	case domain.CodeVersionConflict, domain.CodeStateConflict, domain.CodeDuplicateEvidence, domain.CodeIdempotencyConflict, domain.CodePreviewExpired:
		status = http.StatusConflict
	case domain.CodeUnknownRuleSet:
		status = http.StatusUnprocessableEntity
	case domain.CodeAuditInconsistent:
		status = http.StatusConflict
	case domain.CodeRetryableStorage:
		status = http.StatusServiceUnavailable
	case domain.CodeForbidden:
		status = http.StatusForbidden
	}
	body := errorBody{}
	body.Error.Code, body.Error.Message = string(code), err.Error()
	writeJSON(writer, status, body)
}

func decodeJSON(writer http.ResponseWriter, request *http.Request, target any) error {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return domain.NewError(domain.CodeValidation, "Content-Type 必须为 application/json")
	}
	request.Body = http.MaxBytesReader(writer, request.Body, maxJSONBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return domain.NewError(domain.CodeValidation, "JSON 请求无效：%s", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return domain.NewError(domain.CodeValidation, "请求只能包含一个 JSON 对象")
	}
	return nil
}

func clientSeat(request *http.Request) string {
	return strings.TrimSpace(request.URL.Query().Get("seat"))
}
