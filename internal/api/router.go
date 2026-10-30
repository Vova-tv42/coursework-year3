package api

import (
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
)

type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type server struct {
	pool     *pgxpool.Pool
	sessions *scs.SessionManager
	jobs     *river.Client[pgx.Tx]
	reposDir string
}

func NewRouter(pool *pgxpool.Pool, jobs *river.Client[pgx.Tx], reposDir string, cookieSecure bool, spaFiles fs.FS) http.Handler {
	s := &server{pool: pool, sessions: newSessionManager(pool, cookieSecure), jobs: jobs, reposDir: reposDir}

	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.Recoverer)
	router.Use(middleware.Compress(5))

	router.Route("/api", func(apiRouter chi.Router) {
		apiRouter.Use(http.NewCrossOriginProtection().Handler)
		apiRouter.Use(s.sessions.LoadAndSave)
		apiRouter.NotFound(func(w http.ResponseWriter, r *http.Request) {
			writeError(w, http.StatusNotFound, "not_found", "resource not found")
		})

		apiRouter.Post("/auth/register", s.register)
		apiRouter.Post("/auth/login", s.login)

		apiRouter.Group(func(signedIn chi.Router) {
			signedIn.Use(s.requireSession)
			signedIn.Post("/auth/logout", s.logout)
			signedIn.Get("/auth/me", s.getMe)
			signedIn.Get("/students", s.listStudents)
			signedIn.Post("/students", s.createStudent)
			signedIn.Get("/students/{studentId}", s.getStudent)
			signedIn.Put("/students/{studentId}", s.updateStudent)
			signedIn.Delete("/students/{studentId}", s.deleteStudent)
			signedIn.Get("/repos/{repoId}", s.getRepository)
			signedIn.Post("/repos/{repoId}/analyze", s.analyzeRepository)
		})
	})

	router.Handle("/*", spaHandler(spaFiles))

	return router
}

func readJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "request body must be valid JSON")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorResponse{Code: code, Message: message})
}

func writeInternalError(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("request failed", "method", r.Method, "path", r.URL.Path, "error", err)
	writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
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
