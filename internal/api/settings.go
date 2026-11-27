package api

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"coursework/internal/metrics"
	"coursework/internal/store"
	"coursework/internal/worker"
)

const (
	maxJunkPatterns         = 200
	maxJunkPatternLength    = 500
	maxCommitSizeRanges     = 20
	maxRapidSeriesCommits   = 1000
	maxRapidSeriesSeconds   = 24 * 60 * 60
	minRapidSeriesCommits   = 2
	minRapidSeriesSeconds   = 1
	settingsForbiddenReason = "only coordinators and repository managers can change settings"
)

type junkPatternsSetting struct {
	Patterns []string `json:"patterns"`
}

var defaultJunkPatternsSetting = junkPatternsSetting{Patterns: metrics.DefaultJunkPatterns}

type settingResponse[T any] struct {
	Value     T    `json:"value"`
	IsDefault bool `json:"isDefault"`
}

// globalSetting serves GET, PUT and DELETE (reset to defaults) for one row of the settings table
type globalSetting[T any] struct {
	pool     *pgxpool.Pool
	name     string
	defaults T
	// normalize validates the input and may return it in a canonical form
	normalize func(T) (T, error)
	// onChange runs in the saving transaction, also after a reset
	onChange func(ctx context.Context, tx pgx.Tx, value T) error
}

func (setting globalSetting[T]) register(router chi.Router, path string) {
	router.Get(path, setting.get)
	router.Put(path, setting.save)
	router.Delete(path, setting.reset)
}

func (setting globalSetting[T]) get(w http.ResponseWriter, r *http.Request) {
	value, isDefault, err := store.LoadSetting(r.Context(), setting.pool, setting.name, setting.defaults)
	if err != nil {
		writeInternalError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, settingResponse[T]{Value: value, IsDefault: isDefault})
}

func (setting globalSetting[T]) save(w http.ResponseWriter, r *http.Request) {
	if !canEditGlobalSettings(currentUser(r)) {
		writeError(w, http.StatusForbidden, "forbidden", settingsForbiddenReason)
		return
	}

	var input T
	if !readJSON(w, r, &input) {
		return
	}

	value, err := setting.normalize(input)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_input", err.Error())
		return
	}

	encodedValue, err := json.Marshal(value)
	if err != nil {
		writeInternalError(w, r, err)
		return
	}

	err = pgx.BeginFunc(r.Context(), setting.pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(r.Context(), `
			INSERT INTO settings (name, value) VALUES ($1, $2)
			ON CONFLICT (name) DO UPDATE SET value = EXCLUDED.value`,
			setting.name, encodedValue)
		if err != nil {
			return err
		}

		return setting.runOnChange(r.Context(), tx, value)
	})
	if err != nil {
		writeInternalError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, settingResponse[T]{Value: value})
}

func (setting globalSetting[T]) reset(w http.ResponseWriter, r *http.Request) {
	if !canEditGlobalSettings(currentUser(r)) {
		writeError(w, http.StatusForbidden, "forbidden", settingsForbiddenReason)
		return
	}

	err := pgx.BeginFunc(r.Context(), setting.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(r.Context(), "DELETE FROM settings WHERE name = $1", setting.name); err != nil {
			return err
		}

		return setting.runOnChange(r.Context(), tx, setting.defaults)
	})
	if err != nil {
		writeInternalError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, settingResponse[T]{Value: setting.defaults, IsDefault: true})
}

func (setting globalSetting[T]) runOnChange(ctx context.Context, tx pgx.Tx, value T) error {
	if setting.onChange == nil {
		return nil
	}
	return setting.onChange(ctx, tx, value)
}

func canEditGlobalSettings(viewer user) bool {
	return viewer.Role == roleCoordinator || viewer.Role == roleRepoManager
}

func (s *server) registerSettings(router chi.Router) {
	globalSetting[junkPatternsSetting]{
		pool:      s.pool,
		name:      store.JunkPatternsSetting,
		defaults:  defaultJunkPatternsSetting,
		normalize: normalizeJunkPatterns,
	}.register(router, "/settings/junk-patterns")

	globalSetting[metrics.CommitSizeSettings]{
		pool:      s.pool,
		name:      store.CommitSizesSetting,
		defaults:  metrics.DefaultCommitSizeSettings,
		normalize: normalizeCommitSizeSettings,
	}.register(router, "/settings/commit-sizes")

	globalSetting[metrics.RapidSeriesSettings]{
		pool:      s.pool,
		name:      store.RapidSeriesSetting,
		defaults:  metrics.DefaultRapidSeriesSettings,
		normalize: normalizeRapidSeriesSettings,
		onChange:  recomputeAllRapidSeries,
	}.register(router, "/settings/rapid-series")
}

// recomputeAllRapidSeries locks every repository, so it waits for running analyses to save their results first
func recomputeAllRapidSeries(ctx context.Context, tx pgx.Tx, settings metrics.RapidSeriesSettings) error {
	rows, err := tx.Query(ctx, "SELECT id FROM repositories ORDER BY id FOR UPDATE")
	if err != nil {
		return err
	}

	repositoryIDs, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return err
	}

	for _, repositoryID := range repositoryIDs {
		if err := worker.ReplaceRapidSeries(ctx, tx, repositoryID, settings); err != nil {
			return err
		}
	}

	return nil
}

func normalizeJunkPatterns(setting junkPatternsSetting) (junkPatternsSetting, error) {
	if setting.Patterns == nil {
		return setting, errors.New("patterns must be a list")
	}

	if len(setting.Patterns) > maxJunkPatterns {
		return setting, fmt.Errorf("at most %d patterns are allowed", maxJunkPatterns)
	}

	for _, pattern := range setting.Patterns {
		isBlank := strings.TrimSpace(pattern) == ""
		isValidGlob := pattern == metrics.BinaryPattern || doublestar.ValidatePattern(pattern)

		switch {
		case isBlank:
			return setting, errors.New("patterns must not be empty")
		case len(pattern) > maxJunkPatternLength:
			return setting, fmt.Errorf("a pattern must be at most %d characters", maxJunkPatternLength)
		case !isValidGlob:
			return setting, fmt.Errorf("%q is not a valid glob pattern", pattern)
		}
	}

	return setting, nil
}

// normalizeCommitSizeSettings sorts the ranges by their lower limit, the order of the chart bars
func normalizeCommitSizeSettings(settings metrics.CommitSizeSettings) (metrics.CommitSizeSettings, error) {
	if len(settings.Ranges) == 0 {
		return settings, errors.New("at least one range is required")
	}

	if len(settings.Ranges) > maxCommitSizeRanges {
		return settings, fmt.Errorf("at most %d ranges are allowed", maxCommitSizeRanges)
	}

	isLargeCommitLinesNegative := settings.LargeCommitLines != nil && *settings.LargeCommitLines < 0
	if isLargeCommitLinesNegative {
		return settings, errors.New("the large commit limit must not be negative")
	}

	openRanges := 0
	for _, sizeRange := range settings.Ranges {
		isEndBeforeStart := sizeRange.To != nil && *sizeRange.To < sizeRange.From

		switch {
		case sizeRange.From < 0:
			return settings, errors.New("range limits must not be negative")
		case isEndBeforeStart:
			return settings, fmt.Errorf("range %d-%d ends before it starts", sizeRange.From, *sizeRange.To)
		case sizeRange.To == nil:
			openRanges++
		}
	}

	if openRanges > 1 {
		return settings, errors.New("only one range may have no upper limit")
	}

	settings.Ranges = slices.Clone(settings.Ranges)
	slices.SortFunc(settings.Ranges, func(a, b metrics.CommitSizeRange) int { return cmp.Compare(a.From, b.From) })

	for i := 1; i < len(settings.Ranges); i++ {
		previous, current := settings.Ranges[i-1], settings.Ranges[i]
		isOverlapping := previous.To == nil || *previous.To >= current.From
		if isOverlapping {
			return settings, fmt.Errorf("ranges %s and %s overlap", previous.Label(), current.Label())
		}
	}

	return settings, nil
}

func normalizeRapidSeriesSettings(settings metrics.RapidSeriesSettings) (metrics.RapidSeriesSettings, error) {
	isAnyRuleEnabled := settings.Consecutive.Enabled || settings.Window.Enabled
	if !isAnyRuleEnabled {
		return settings, errors.New("at least one rule must be enabled")
	}

	if err := validateRapidSeriesRule(settings.Consecutive.MinCommits, settings.Consecutive.MaxGapSeconds); err != nil {
		return settings, err
	}

	return settings, validateRapidSeriesRule(settings.Window.MinCommits, settings.Window.WindowSeconds)
}

func validateRapidSeriesRule(minCommits, seconds int) error {
	isCommitCountOutOfRange := minCommits < minRapidSeriesCommits || minCommits > maxRapidSeriesCommits
	isDurationOutOfRange := seconds < minRapidSeriesSeconds || seconds > maxRapidSeriesSeconds

	switch {
	case isCommitCountOutOfRange:
		return fmt.Errorf("the number of commits must be between %d and %d", minRapidSeriesCommits, maxRapidSeriesCommits)
	case isDurationOutOfRange:
		return fmt.Errorf("the time must be between %d and %d seconds", minRapidSeriesSeconds, maxRapidSeriesSeconds)
	}

	return nil
}
