package api

import (
	"net/http"

	"coursework/internal/metrics"
)

type checklistResponse struct {
	Evaluated bool `json:"evaluated"`
	metrics.Checklist
}

func (s *server) getChecklist(w http.ResponseWriter, r *http.Request) {
	repo, found := s.loadVisibleRepository(w, r)
	if !found {
		return
	}

	var checklist *metrics.Checklist
	err := s.pool.QueryRow(r.Context(), "SELECT checklist FROM repositories WHERE id = $1", repo.ID).Scan(&checklist)
	if err != nil {
		writeInternalError(w, r, err)
		return
	}

	if checklist == nil {
		writeJSON(w, http.StatusOK, checklistResponse{Checklist: metrics.Checklist{Items: []metrics.ChecklistItem{}}})
		return
	}

	writeJSON(w, http.StatusOK, checklistResponse{Evaluated: true, Checklist: *checklist})
}
