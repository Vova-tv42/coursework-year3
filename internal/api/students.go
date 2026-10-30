package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"coursework/internal/worker"
)

const maxFullNameLength = 200

// $1 = viewer role, $2 = viewer id, $3 = viewer stream (NULL unless coordinator).
const visibleStudentsQuery = `
	SELECT s.id, s.full_name, s.stream, s.supervisor_id, supervisor.display_name,
		r.id, r.clone_url, r.default_branch, r.status, r.status_reason,
		r.analysis_started_at, r.analysis_finished_at, r.last_commit_at, r.hosting_metadata
	FROM students s
	JOIN users supervisor ON supervisor.id = s.supervisor_id
	JOIN repositories r ON r.student_id = s.id
	WHERE ($1 = 'repo_manager'
		OR ($1 = 'supervisor' AND s.supervisor_id = $2)
		OR ($1 = 'coordinator' AND s.stream = $3))`

var (
	scpCloneURLPattern = regexp.MustCompile(`^[\w.-]+@[\w.-]+:\S+$`)
	cloneURLSchemes    = []string{"http", "https", "git"}
)

type studentInput struct {
	FullName string `json:"fullName"`
	Stream   string `json:"stream"`
	CloneURL string `json:"cloneUrl"`
}

type studentOverview struct {
	ID             int64      `json:"id"`
	FullName       string     `json:"fullName"`
	Stream         string     `json:"stream"`
	SupervisorName string     `json:"supervisorName"`
	CanEdit        bool       `json:"canEdit"`
	Repository     repository `json:"repository"`
}

type repository struct {
	ID                 int64                   `json:"id"`
	StudentID          int64                   `json:"studentId"`
	CloneURL           string                  `json:"cloneUrl"`
	DefaultBranch      *string                 `json:"defaultBranch"`
	Status             string                  `json:"status"`
	StatusReason       *string                 `json:"statusReason"`
	AnalysisStartedAt  *time.Time              `json:"analysisStartedAt"`
	AnalysisFinishedAt *time.Time              `json:"analysisFinishedAt"`
	LastCommitAt       *time.Time              `json:"lastCommitAt"`
	Hosting            *worker.HostingMetadata `json:"hosting"`
}

func (s *server) listStudents(w http.ResponseWriter, r *http.Request) {
	viewer := currentUser(r)
	var streamFilter *string
	if stream := r.URL.Query().Get("stream"); stream != "" {
		streamFilter = &stream
	}

	rows, err := s.pool.Query(r.Context(),
		visibleStudentsQuery+" AND ($4::text IS NULL OR s.stream::text = $4) ORDER BY s.full_name, s.id",
		viewer.Role, viewer.ID, viewer.Stream, streamFilter)
	if err != nil {
		writeInternalError(w, r, err)
		return
	}
	students, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (studentOverview, error) {
		return scanStudent(row, viewer)
	})
	if err != nil {
		writeInternalError(w, r, err)
		return
	}

	if students == nil {
		students = []studentOverview{}
	}

	writeJSON(w, http.StatusOK, students)
}

func (s *server) getStudent(w http.ResponseWriter, r *http.Request) {
	studentID, err := strconv.ParseInt(chi.URLParam(r, "studentId"), 10, 64)
	if err != nil {
		writeStudentNotFound(w)
		return
	}

	student, err := s.findVisibleStudent(r.Context(), currentUser(r), studentID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeStudentNotFound(w)
		return
	}
	if err != nil {
		writeInternalError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, student)
}

func (s *server) createStudent(w http.ResponseWriter, r *http.Request) {
	viewer := currentUser(r)
	if viewer.Role != roleSupervisor {
		writeError(w, http.StatusForbidden, "forbidden", "only supervisors can add students")
		return
	}

	var input studentInput
	if !readJSON(w, r, &input) {
		return
	}
	if err := validateStudentInput(input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_input", err.Error())
		return
	}

	var studentID int64
	err := s.pool.QueryRow(r.Context(), `
		WITH new_student AS (
			INSERT INTO students (supervisor_id, full_name, stream) VALUES ($1, $2, $3) RETURNING id
		)
		INSERT INTO repositories (student_id, clone_url)
		SELECT id, $4 FROM new_student
		RETURNING student_id`,
		viewer.ID, input.FullName, input.Stream, input.CloneURL).Scan(&studentID)
	if err != nil {
		writeInternalError(w, r, err)
		return
	}

	student, err := s.findVisibleStudent(r.Context(), viewer, studentID)
	if err != nil {
		writeInternalError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, student)
}

func (s *server) updateStudent(w http.ResponseWriter, r *http.Request) {
	viewer := currentUser(r)
	studentID, err := strconv.ParseInt(chi.URLParam(r, "studentId"), 10, 64)
	if err != nil {
		writeStudentNotFound(w)
		return
	}

	var input studentInput
	if !readJSON(w, r, &input) {
		return
	}
	if err := validateStudentInput(input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_input", err.Error())
		return
	}

	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		writeInternalError(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	result, err := tx.Exec(r.Context(),
		"UPDATE students SET full_name = $1, stream = $2 WHERE id = $3 AND supervisor_id = $4",
		input.FullName, input.Stream, studentID, viewer.ID)
	if err != nil {
		writeInternalError(w, r, err)
		return
	}
	if result.RowsAffected() == 0 {
		writeStudentNotFound(w)
		return
	}

	repositoryID, isCloneURLChanged, err := resetRepositoryIfCloneURLChanged(r.Context(), tx, studentID, input.CloneURL)
	if err != nil {
		writeInternalError(w, r, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeInternalError(w, r, err)
		return
	}
	if isCloneURLChanged {
		s.removeClone(repositoryID)
	}

	student, err := s.findVisibleStudent(r.Context(), viewer, studentID)
	if err != nil {
		writeInternalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, student)
}

func resetRepositoryIfCloneURLChanged(ctx context.Context, tx pgx.Tx, studentID int64, cloneURL string) (repositoryID int64, isChanged bool, err error) {
	err = tx.QueryRow(ctx, `
		UPDATE repositories
		SET clone_url = $2, status = 'never', status_reason = NULL, default_branch = NULL,
			hosting_metadata = NULL, analysis_started_at = NULL, analysis_finished_at = NULL,
			last_analyzed_head = NULL, clone_size_bytes = NULL, checklist = NULL, last_commit_at = NULL
		WHERE student_id = $1 AND clone_url <> $2
		RETURNING id`,
		studentID, cloneURL).Scan(&repositoryID)
	isCloneURLUnchanged := errors.Is(err, pgx.ErrNoRows)
	if isCloneURLUnchanged {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("reset repository: %w", err)
	}

	for _, table := range []string{"commits", "snapshot_files", "rapid_series"} {
		if _, err := tx.Exec(ctx, "DELETE FROM "+table+" WHERE repository_id = $1", repositoryID); err != nil {
			return 0, false, fmt.Errorf("delete %s: %w", table, err)
		}
	}
	return repositoryID, true, nil
}

func (s *server) deleteStudent(w http.ResponseWriter, r *http.Request) {
	studentID, err := strconv.ParseInt(chi.URLParam(r, "studentId"), 10, 64)
	if err != nil {
		writeStudentNotFound(w)
		return
	}

	var repositoryID int64
	err = s.pool.QueryRow(r.Context(), `
		DELETE FROM students s
		USING repositories r
		WHERE r.student_id = s.id AND s.id = $1 AND s.supervisor_id = $2
		RETURNING r.id`,
		studentID, currentUser(r).ID).Scan(&repositoryID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeStudentNotFound(w)
		return
	}

	if err != nil {
		writeInternalError(w, r, err)
		return
	}

	s.removeClone(repositoryID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) findVisibleStudent(ctx context.Context, viewer user, studentID int64) (studentOverview, error) {
	row := s.pool.QueryRow(ctx, visibleStudentsQuery+" AND s.id = $4",
		viewer.Role, viewer.ID, viewer.Stream, studentID)
	return scanStudent(row, viewer)
}

func scanStudent(row pgx.Row, viewer user) (studentOverview, error) {
	var student studentOverview
	var supervisorID int64
	repo := &student.Repository
	err := row.Scan(&student.ID, &student.FullName, &student.Stream, &supervisorID, &student.SupervisorName,
		&repo.ID, &repo.CloneURL, &repo.DefaultBranch, &repo.Status, &repo.StatusReason,
		&repo.AnalysisStartedAt, &repo.AnalysisFinishedAt, &repo.LastCommitAt, &repo.Hosting)

	repo.StudentID = student.ID
	student.CanEdit = supervisorID == viewer.ID
	return student, err
}

func writeStudentNotFound(w http.ResponseWriter) {
	writeError(w, http.StatusNotFound, "not_found", "student not found")
}

func validateStudentInput(input studentInput) error {
	fullNameLength := utf8.RuneCountInString(input.FullName)

	switch {
	case fullNameLength == 0 || fullNameLength > maxFullNameLength:
		return fmt.Errorf("full name must be 1-%d characters", maxFullNameLength)
	case !slices.Contains(streams, input.Stream):
		return errors.New("stream must be bachelor or master")
	case !isValidCloneURL(input.CloneURL):
		return errors.New("clone URL must be an http(s):// or git:// URL, or git@host:path")
	}
	return nil
}

func isValidCloneURL(cloneURL string) bool {
	if scpCloneURLPattern.MatchString(cloneURL) {
		return true
	}

	parsedURL, err := url.Parse(cloneURL)
	if err != nil {
		return false
	}
	isSupportedScheme := slices.Contains(cloneURLSchemes, parsedURL.Scheme)
	hasRepositoryPath := strings.Trim(parsedURL.Path, "/") != ""
	return isSupportedScheme && parsedURL.Host != "" && hasRepositoryPath
}
