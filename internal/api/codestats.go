package api

import (
	"net/http"

	"github.com/jackc/pgx/v5"

	"coursework/internal/metrics"
	"coursework/internal/store"
)

// getCodeStats applies the junk patterns at read time, so a changed list needs no new analysis
func (s *server) getCodeStats(w http.ResponseWriter, r *http.Request) {
	repo, found := s.loadVisibleRepository(w, r)
	if !found {
		return
	}

	junkPatterns, _, err := store.LoadSetting(r.Context(), s.pool, store.JunkPatternsSetting, defaultJunkPatternsSetting)
	if err != nil {
		writeInternalError(w, r, err)
		return
	}

	rows, err := s.pool.Query(r.Context(), `
		SELECT path, size_bytes, is_binary, category, code_lines, comment_lines
		FROM snapshot_files
		WHERE repository_id = $1
		ORDER BY path`, repo.ID)
	if err != nil {
		writeInternalError(w, r, err)
		return
	}

	files, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (metrics.SnapshotFile, error) {
		var file metrics.SnapshotFile
		err := row.Scan(&file.Path, &file.SizeBytes, &file.IsBinary, &file.Category, &file.CodeLines, &file.CommentLines)
		return file, err
	})
	if err != nil {
		writeInternalError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, metrics.CodeStats(files, junkPatterns.Patterns))
}
