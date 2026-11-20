package api

import (
	"context"
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"coursework/internal/git"
	"coursework/internal/metrics"
	"coursework/internal/worker"
)

type fileModifications struct {
	Path           string    `json:"path"`
	Modifications  int       `json:"modifications"`
	LinesAdded     int       `json:"linesAdded"`
	LinesDeleted   int       `json:"linesDeleted"`
	LastModifiedAt time.Time `json:"lastModifiedAt"`
}

type commitFile struct {
	Path         string  `json:"path"`
	OldPath      *string `json:"oldPath"`
	LinesAdded   int     `json:"linesAdded"`
	LinesDeleted int     `json:"linesDeleted"`
	IsBinary     bool    `json:"isBinary"`
}

type commitDetails struct {
	metrics.CommitSummary
	Files []commitFile `json:"files"`
}

type commitDiff struct {
	Diff      string `json:"diff"`
	Truncated bool   `json:"truncated"`
}

const commitSummaryColumns = `c.sha, c.author_name, c.author_email, c.authored_at, c.message, c.is_merge, c.lines_added, c.lines_deleted`

// getTopFiles keeps junk files, because a reviewer may want to see a committed node_modules too
func (s *server) getTopFiles(w http.ResponseWriter, r *http.Request) {
	repo, found := s.loadVisibleRepository(w, r)
	if !found {
		return
	}

	rows, err := s.pool.Query(r.Context(), `
		SELECT fc.path, count(*), sum(fc.lines_added), sum(fc.lines_deleted), max(c.authored_at)
		FROM file_changes fc
		JOIN commits c ON c.repository_id = fc.repository_id AND c.sha = fc.sha
		WHERE fc.repository_id = $1
		GROUP BY fc.path
		ORDER BY count(*) DESC, fc.path`, repo.ID)
	if err != nil {
		writeInternalError(w, r, err)
		return
	}

	files, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (fileModifications, error) {
		var file fileModifications
		err := row.Scan(&file.Path, &file.Modifications, &file.LinesAdded, &file.LinesDeleted, &file.LastModifiedAt)
		return file, err
	})
	if err != nil {
		writeInternalError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, nonNil(files))
}

func (s *server) getFileCommits(w http.ResponseWriter, r *http.Request) {
	repo, found := s.loadVisibleRepository(w, r)
	if !found {
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		writeError(w, http.StatusBadRequest, "invalid_input", "path is required")
		return
	}

	rows, err := s.pool.Query(r.Context(), `
		SELECT `+commitSummaryColumns+`
		FROM commits c
		JOIN file_changes fc ON fc.repository_id = c.repository_id AND fc.sha = c.sha
		WHERE c.repository_id = $1 AND fc.path = $2
		ORDER BY c.authored_at DESC, c.committed_at DESC, c.sha`, repo.ID, path)
	if err != nil {
		writeInternalError(w, r, err)
		return
	}

	commits, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (metrics.CommitSummary, error) {
		return scanCommitSummary(row)
	})
	if err != nil {
		writeInternalError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, nonNil(commits))
}

func (s *server) getCommit(w http.ResponseWriter, r *http.Request) {
	repo, found := s.loadVisibleRepository(w, r)
	if !found {
		return
	}

	summary, found := s.loadRepositoryCommit(w, r, repo.ID)
	if !found {
		return
	}

	rows, err := s.pool.Query(r.Context(), `
		SELECT path, old_path, lines_added, lines_deleted, is_binary
		FROM file_changes
		WHERE repository_id = $1 AND sha = $2
		ORDER BY path`, repo.ID, summary.SHA)
	if err != nil {
		writeInternalError(w, r, err)
		return
	}

	files, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (commitFile, error) {
		var file commitFile
		err := row.Scan(&file.Path, &file.OldPath, &file.LinesAdded, &file.LinesDeleted, &file.IsBinary)
		return file, err
	})
	if err != nil {
		writeInternalError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, commitDetails{CommitSummary: summary, Files: nonNil(files)})
}

// getCommitDiff runs git only for a commit stored for this repository, so the SHA never reaches git unchecked
func (s *server) getCommitDiff(w http.ResponseWriter, r *http.Request) {
	repo, found := s.loadVisibleRepository(w, r)
	if !found {
		return
	}

	summary, found := s.loadRepositoryCommit(w, r, repo.ID)
	if !found {
		return
	}

	cloneDir := worker.ClonePath(s.reposDir, repo.ID)
	if _, err := os.Stat(cloneDir); errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusNotFound, "not_found", "repository clone is not available, run the analysis again")
		return
	}

	// The clone is local, so git needs no access token here
	diff, truncated, err := git.Show(r.Context(), git.ExecRunner{}, cloneDir, summary.SHA)
	if err != nil {
		writeInternalError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, commitDiff{Diff: diff, Truncated: truncated})
}

// loadRepositoryCommit reads the sha URL parameter and writes 404 or 500 itself when it returns false
func (s *server) loadRepositoryCommit(w http.ResponseWriter, r *http.Request, repositoryID int64) (metrics.CommitSummary, bool) {
	summary, err := s.findCommit(r.Context(), repositoryID, chi.URLParam(r, "sha"))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "commit not found")
		return summary, false
	}

	if err != nil {
		writeInternalError(w, r, err)
		return summary, false
	}

	return summary, true
}

func (s *server) findCommit(ctx context.Context, repositoryID int64, sha string) (metrics.CommitSummary, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT `+commitSummaryColumns+`
		FROM commits c
		WHERE c.repository_id = $1 AND c.sha = $2`, repositoryID, sha)

	return scanCommitSummary(row)
}

func scanCommitSummary(row pgx.Row) (metrics.CommitSummary, error) {
	var commit metrics.CommitSummary
	err := row.Scan(&commit.SHA, &commit.AuthorName, &commit.AuthorEmail, &commit.AuthoredAt,
		&commit.Message, &commit.IsMerge, &commit.LinesAdded, &commit.LinesDeleted)

	return commit, err
}

// nonNil makes an empty result encode as [] instead of null
func nonNil[T any](items []T) []T {
	if items == nil {
		return []T{}
	}

	return items
}
