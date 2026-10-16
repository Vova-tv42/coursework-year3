package api

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func NewRouter(spaFiles fs.FS) http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.Recoverer)
	router.Use(middleware.Compress(5))

	router.Route("/api", func(apiRouter chi.Router) {
		apiRouter.NotFound(func(w http.ResponseWriter, r *http.Request) {
			writeError(w, http.StatusNotFound, "not_found", "resource not found")
		})
	})

	router.Handle("/*", spaHandler(spaFiles))

	return router
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorResponse{Code: code, Message: message})
}

func spaHandler(spaFiles fs.FS) http.HandlerFunc {
	fileServer := http.FileServerFS(spaFiles)

	return func(w http.ResponseWriter, r *http.Request) {
		filePath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		fileInfo, err := fs.Stat(spaFiles, filePath)
		isExistingFile := err == nil && !fileInfo.IsDir()

		if !isExistingFile {
			w.Header().Set("Cache-Control", "no-store")
			http.ServeFileFS(w, r, spaFiles, "index.html")
			return
		}

		if strings.HasPrefix(filePath, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		fileServer.ServeHTTP(w, r)
	}
}
