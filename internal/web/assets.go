package web

import (
	"io/fs"
	"net/http"
)

func serveStatic(writer http.ResponseWriter, name, contentType string) {
	raw, err := fs.ReadFile(staticFiles, "static/"+name)
	if err != nil {
		http.Error(writer, "资源不存在", http.StatusNotFound)
		return
	}
	writer.Header().Set("Content-Type", contentType)
	writer.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = writer.Write(raw)
}

func (s *Server) HandleWorkbench(writer http.ResponseWriter, request *http.Request) {
	if request.URL.Path != "/" {
		http.NotFound(writer, request)
		return
	}
	serveStatic(writer, "index.html", "text/html; charset=utf-8")
}

func (s *Server) HandleCSS(writer http.ResponseWriter, _ *http.Request) {
	serveStatic(writer, "app.css", "text/css; charset=utf-8")
}
func (s *Server) HandleExtensionsCSS(writer http.ResponseWriter, _ *http.Request) {
	serveStatic(writer, "extensions.css", "text/css; charset=utf-8")
}
func (s *Server) HandleJavaScript(writer http.ResponseWriter, _ *http.Request) {
	serveStatic(writer, "app.js", "text/javascript; charset=utf-8")
}

func (s *Server) HandleHealth(writer http.ResponseWriter, _ *http.Request) {
	if !s.service.Healthy() {
		writeJSON(writer, http.StatusServiceUnavailable, map[string]any{"status": "recovering"})
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"status": "ready", "storage": s.service.RecoveryStatus()})
}
