package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"coursework/internal/metrics"
)

const maxPeriodNameLength = 100

const calendarPeriodColumns = `id, name, kind, stream,
	to_char(start_date, 'YYYY-MM-DD'), to_char(end_date, 'YYYY-MM-DD')`

type calendarPeriodInput struct {
	Name      string  `json:"name"`
	StartDate *string `json:"startDate"`
	EndDate   *string `json:"endDate"`
}

func (s *server) listCalendarPeriods(w http.ResponseWriter, r *http.Request) {
	periods, err := s.loadCalendarPeriods(r.Context(), nil)
	if err != nil {
		writeInternalError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, periods)
}

func (s *server) updateCalendarPeriod(w http.ResponseWriter, r *http.Request) {
	if !canEditGlobalSettings(currentUser(r)) {
		writeError(w, http.StatusForbidden, "forbidden", "only coordinators and repository managers can edit the calendar")
		return
	}

	periodID, err := strconv.ParseInt(chi.URLParam(r, "periodId"), 10, 64)
	if err != nil {
		writePeriodNotFound(w)
		return
	}

	var input calendarPeriodInput
	if !readJSON(w, r, &input) {
		return
	}

	input.Name = strings.TrimSpace(input.Name)
	if err := validateCalendarPeriodInput(input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_input", err.Error())
		return
	}

	rows, err := s.pool.Query(r.Context(), `
		UPDATE calendar_periods SET name = $2, start_date = $3, end_date = $4
		WHERE id = $1
		RETURNING `+calendarPeriodColumns,
		periodID, input.Name, input.StartDate, input.EndDate)
	if err != nil {
		writeInternalError(w, r, err)
		return
	}

	period, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByPos[metrics.CalendarPeriod])
	if errors.Is(err, pgx.ErrNoRows) {
		writePeriodNotFound(w)
		return
	}

	if err != nil {
		writeInternalError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, period)
}

// loadCalendarPeriods returns every period when repositoryID is nil, otherwise only the periods of the student's stream
func (s *server) loadCalendarPeriods(ctx context.Context, repositoryID *int64) ([]metrics.CalendarPeriod, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+calendarPeriodColumns+`
		FROM calendar_periods
		WHERE $1::bigint IS NULL
			OR stream = 'all'
			OR stream::text = (
				SELECT s.stream::text FROM students s JOIN repositories r ON r.student_id = s.id WHERE r.id = $1)
		ORDER BY start_date NULLS LAST, id`,
		repositoryID)
	if err != nil {
		return nil, err
	}

	return pgx.CollectRows(rows, pgx.RowToStructByPos[metrics.CalendarPeriod])
}

func validateCalendarPeriodInput(input calendarPeriodInput) error {
	nameLength := utf8.RuneCountInString(input.Name)
	hasStartDate := input.StartDate != nil
	hasEndDate := input.EndDate != nil

	switch {
	case nameLength == 0 || nameLength > maxPeriodNameLength:
		return fmt.Errorf("name must be 1-%d characters", maxPeriodNameLength)

	case hasStartDate != hasEndDate:
		return errors.New("start and end dates must be both set or both empty")

	case !hasStartDate:
		return nil
	}

	startDate, startErr := time.Parse(time.DateOnly, *input.StartDate)
	endDate, endErr := time.Parse(time.DateOnly, *input.EndDate)

	switch {
	case startErr != nil || endErr != nil:
		return errors.New("dates must be in YYYY-MM-DD format")

	case endDate.Before(startDate):
		return errors.New("end date must not be before the start date")
	}

	return nil
}

func writePeriodNotFound(w http.ResponseWriter) {
	writeError(w, http.StatusNotFound, "not_found", "calendar period not found")
}
