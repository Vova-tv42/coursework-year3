package api

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5"

	"coursework/internal/git"
	"coursework/internal/metrics"
)

type activityResponse struct {
	TimeZone    string                      `json:"timeZone"`
	Days        []metrics.DayActivity       `json:"days"`
	RapidSeries []metrics.RapidCommitSeries `json:"rapidSeries"`
}

func (s *server) getActivity(w http.ResponseWriter, r *http.Request) {
	repo, found := s.loadVisibleRepository(w, r)
	if !found {
		return
	}

	commits, err := s.loadCommits(r.Context(), repo.ID)
	if err != nil {
		writeInternalError(w, r, err)
		return
	}

	rapidSeries, err := s.loadRapidSeries(r.Context(), repo.ID, commits)
	if err != nil {
		writeInternalError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, activityResponse{
		TimeZone:    metrics.TimeZone,
		Days:        metrics.DailyActivity(commits),
		RapidSeries: rapidSeries,
	})
}

func (s *server) getCommitSizes(w http.ResponseWriter, r *http.Request) {
	repo, found := s.loadVisibleRepository(w, r)
	if !found {
		return
	}

	commits, err := s.loadCommits(r.Context(), repo.ID)
	if err != nil {
		writeInternalError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, metrics.CommitSizes(commits))
}

// loadCommits returns commits newest first, the order the metrics expect from git log
func (s *server) loadCommits(ctx context.Context, repositoryID int64) ([]git.Commit, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT sha, author_name, author_email, authored_at, committed_at, message, is_merge, lines_added, lines_deleted
		FROM commits
		WHERE repository_id = $1
		ORDER BY authored_at DESC, committed_at DESC, sha`,
		repositoryID)

	if err != nil {
		return nil, err
	}

	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (git.Commit, error) {
		var commit git.Commit
		err := row.Scan(&commit.SHA, &commit.AuthorName, &commit.AuthorEmail, &commit.AuthoredAt, &commit.CommittedAt,
			&commit.Message, &commit.IsMerge, &commit.LinesAdded, &commit.LinesDeleted)

		return commit, err
	})
}

// loadRapidSeries reads the series saved by the worker, which keep the commit order of git log
func (s *server) loadRapidSeries(ctx context.Context, repositoryID int64, commits []git.Commit) ([]metrics.RapidCommitSeries, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT commit_shas, started_at, ended_at
		FROM rapid_series
		WHERE repository_id = $1
		ORDER BY started_at, id`,
		repositoryID)

	if err != nil {
		return nil, err
	}

	commitsBySHA := make(map[string]git.Commit, len(commits))
	for _, commit := range commits {
		commitsBySHA[commit.SHA] = commit
	}

	rapidSeries, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (metrics.RapidCommitSeries, error) {
		var series metrics.RapidCommitSeries
		var commitSHAs []string
		if err := row.Scan(&commitSHAs, &series.StartedAt, &series.EndedAt); err != nil {
			return series, err
		}

		series.Commits = make([]metrics.CommitSummary, 0, len(commitSHAs))
		for _, sha := range commitSHAs {
			// A refresh may replace the history between the two queries; such commits are skipped
			if commit, found := commitsBySHA[sha]; found {
				series.Commits = append(series.Commits, metrics.Summarize(commit))
			}
		}

		return series, nil
	})

	if err != nil {
		return nil, err
	}

	if rapidSeries == nil {
		rapidSeries = []metrics.RapidCommitSeries{}
	}

	return rapidSeries, nil
}
