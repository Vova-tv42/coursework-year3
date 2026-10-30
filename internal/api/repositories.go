package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"coursework/internal/worker"
)

func (s *server) getRepository(w http.ResponseWriter, r *http.Request) {
	repo, found := s.loadVisibleRepository(w, r)
	if !found {
		return
	}

	writeJSON(w, http.StatusOK, repo)
}

func (s *server) analyzeRepository(w http.ResponseWriter, r *http.Request) {
	repo, found := s.loadVisibleRepository(w, r)
	if !found {
		return
	}

	if err := worker.EnqueueAnalysis(r.Context(), s.pool, s.jobs, repo.ID); err != nil {
		writeInternalError(w, r, err)
		return
	}

	repo, err := s.findVisibleRepository(r.Context(), currentUser(r), repo.ID)
	if err != nil {
		writeInternalError(w, r, err)
		return
	}

	writeJSON(w, http.StatusAccepted, repo)
}

// loadVisibleRepository reads the repoId URL parameter and writes 404 or 500 itself when it returns false
func (s *server) loadVisibleRepository(w http.ResponseWriter, r *http.Request) (repository, bool) {
	repositoryID, err := strconv.ParseInt(chi.URLParam(r, "repoId"), 10, 64)
	if err != nil {
		writeRepositoryNotFound(w)
		return repository{}, false
	}

	repo, err := s.findVisibleRepository(r.Context(), currentUser(r), repositoryID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeRepositoryNotFound(w)
		return repository{}, false
	}

	if err != nil {
		writeInternalError(w, r, err)
		return repository{}, false
	}

	return repo, true
}

func (s *server) findVisibleRepository(ctx context.Context, viewer user, repositoryID int64) (repository, error) {
	row := s.pool.QueryRow(ctx, visibleStudentsQuery+" AND r.id = $4",
		viewer.Role, viewer.ID, viewer.Stream, repositoryID)
	student, err := scanStudent(row, viewer)

	return student.Repository, err
}

// removeClone runs after the database change is committed, so a failure is only logged
func (s *server) removeClone(repositoryID int64) {
	if err := os.RemoveAll(worker.ClonePath(s.reposDir, repositoryID)); err != nil {
		slog.Error("remove repository clone", "repositoryId", repositoryID, "error", err)
	}
}

func writeRepositoryNotFound(w http.ResponseWriter) {
	writeError(w, http.StatusNotFound, "not_found", "repository not found")
}
