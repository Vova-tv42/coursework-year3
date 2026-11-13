package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"coursework/internal/metrics"
)

const (
	maxJunkPatterns      = 200
	maxJunkPatternLength = 500
)

type junkPatternsInput struct {
	Patterns []string `json:"patterns"`
}

type junkPatternsResponse struct {
	Patterns  []string `json:"patterns"`
	IsDefault bool     `json:"isDefault"`
}

// loadJunkPatterns returns the defaults when the user has no stored list (NULL)
func (s *server) loadJunkPatterns(ctx context.Context, userID int64) (junkPatternsResponse, error) {
	var patterns []string
	err := s.pool.QueryRow(ctx, "SELECT junk_patterns FROM users WHERE id = $1", userID).Scan(&patterns)
	if err != nil {
		return junkPatternsResponse{}, err
	}

	if patterns == nil {
		return junkPatternsResponse{Patterns: metrics.DefaultJunkPatterns, IsDefault: true}, nil
	}
	return junkPatternsResponse{Patterns: patterns}, nil
}

func (s *server) getJunkPatterns(w http.ResponseWriter, r *http.Request) {
	junkPatterns, err := s.loadJunkPatterns(r.Context(), currentUser(r).ID)
	if err != nil {
		writeInternalError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, junkPatterns)
}

func (s *server) setJunkPatterns(w http.ResponseWriter, r *http.Request) {
	var input junkPatternsInput
	if !readJSON(w, r, &input) {
		return
	}

	if err := validateJunkPatterns(input.Patterns); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_input", err.Error())
		return
	}

	_, err := s.pool.Exec(r.Context(), "UPDATE users SET junk_patterns = $2 WHERE id = $1", currentUser(r).ID, input.Patterns)
	if err != nil {
		writeInternalError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, junkPatternsResponse{Patterns: input.Patterns})
}

func (s *server) resetJunkPatterns(w http.ResponseWriter, r *http.Request) {
	_, err := s.pool.Exec(r.Context(), "UPDATE users SET junk_patterns = NULL WHERE id = $1", currentUser(r).ID)
	if err != nil {
		writeInternalError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, junkPatternsResponse{Patterns: metrics.DefaultJunkPatterns, IsDefault: true})
}

func validateJunkPatterns(patterns []string) error {
	if patterns == nil {
		return errors.New("patterns must be a list")
	}

	if len(patterns) > maxJunkPatterns {
		return fmt.Errorf("at most %d patterns are allowed", maxJunkPatterns)
	}

	for _, pattern := range patterns {
		isBlank := strings.TrimSpace(pattern) == ""
		isValidGlob := pattern == metrics.BinaryPattern || doublestar.ValidatePattern(pattern)

		switch {
		case isBlank:
			return errors.New("patterns must not be empty")
		case len(pattern) > maxJunkPatternLength:
			return fmt.Errorf("a pattern must be at most %d characters", maxJunkPatternLength)
		case !isValidGlob:
			return fmt.Errorf("%q is not a valid glob pattern", pattern)
		}
	}

	return nil
}
