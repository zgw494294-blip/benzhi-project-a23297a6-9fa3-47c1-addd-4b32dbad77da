package web

import (
	"embed"
	"net/http"
	"strings"

	"wildframe/internal/application"
)

//go:embed static/*
var staticFiles embed.FS

type Server struct {
	service *application.Service
	mux     *http.ServeMux
}

func NewServer(service *application.Service) *Server {
	server := &Server{service: service, mux: http.NewServeMux()}
	server.routes()
	return server
}

func (s *Server) Handler() http.Handler { return securityHeaders(s.mux) }

func (s *Server) routes() {
	s.mux.HandleFunc("GET /", s.HandleWorkbench)
	s.mux.HandleFunc("GET /assets/app.css", s.HandleCSS)
	s.mux.HandleFunc("GET /assets/extensions.css", s.HandleExtensionsCSS)
	s.mux.HandleFunc("GET /assets/app.js", s.HandleJavaScript)
	s.mux.HandleFunc("GET /healthz", s.HandleHealth)
	s.mux.HandleFunc("GET /api/v1/collections", s.HandleListCollections)
	s.mux.HandleFunc("POST /api/v1/collections", s.HandleCreateCollection)
	s.mux.HandleFunc("GET /api/v1/collections/{collectionID}", s.HandleGetCollection)
	s.mux.HandleFunc("POST /api/v1/collections/{collectionID}/evidence", s.HandleRegisterEvidence)
	s.mux.HandleFunc("POST /api/v1/collections/{collectionID}/evidence/batch", s.HandleRegisterEvidenceBatch)
	s.mux.HandleFunc("GET /api/v1/collections/{collectionID}/blobs/{blobKey}", s.HandleBlob)
	s.mux.HandleFunc("POST /api/v1/collections/{collectionID}/annotations", s.HandleSubmitAnnotation)
	s.mux.HandleFunc("POST /api/v1/collections/{collectionID}/adjudications", s.HandleAdjudicate)
	s.mux.HandleFunc("POST /api/v1/collections/{collectionID}/adjudications/preview", s.HandlePreviewAdjudication)
	s.mux.HandleFunc("POST /api/v1/collections/{collectionID}/quality-runs", s.HandleRecalculateQuality)
	s.mux.HandleFunc("GET /api/v1/collections/{collectionID}/audit", s.HandleAuditQuery)
	s.mux.HandleFunc("POST /api/v1/collections/{collectionID}/review", s.HandleReview)
	s.mux.HandleFunc("GET /api/v1/collections/{collectionID}/manifest", s.HandleManifestPreview)
	s.mux.HandleFunc("POST /api/v1/collections/{collectionID}/freeze", s.HandleFreeze)
	s.mux.HandleFunc("POST /api/v1/collections/{collectionID}/credential", s.HandleIssueCredential)
	s.mux.HandleFunc("POST /api/v1/credentials/verify", s.HandleVerifyCredential)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.Header().Set("X-Frame-Options", "DENY")
		writer.Header().Set("Referrer-Policy", "no-referrer")
		writer.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' blob: data:; style-src 'self'; script-src 'self'; connect-src 'self'")
		writer.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if strings.HasPrefix(request.URL.Path, "/api/") {
			writer.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(writer, request)
	})
}
