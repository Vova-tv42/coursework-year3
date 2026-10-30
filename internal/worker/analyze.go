package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"

	"coursework/internal/git"
	"coursework/internal/metrics"
)

const (
	analysisWorkers = 2
	analysisTimeout = 30 * time.Minute
	refreshInterval = 24 * time.Hour
	bytesPerKB      = 1024
)

var (
	errSizeLimitExceeded = errors.New("size limit exceeded")
	// errRepositoryChanged means the student was removed or got a new clone URL while the job ran
	errRepositoryChanged = errors.New("repository changed during analysis")
)

type Config struct {
	ReposDir       string
	SizeLimitBytes int64
	GitToken       string
}

type AnalyzeArgs struct {
	RepositoryID int64 `json:"repositoryId"`
}

func (AnalyzeArgs) Kind() string { return "analyze_repository" }

// The default ByState includes completed jobs, which would block every later analysis
func (AnalyzeArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		UniqueOpts: river.UniqueOpts{
			ByArgs: true,
			ByState: []rivertype.JobState{
				rivertype.JobStateAvailable,
				rivertype.JobStatePending,
				rivertype.JobStateRunning,
				rivertype.JobStateScheduled,
				rivertype.JobStateRetryable,
			},
		},
	}
}

type RefreshArgs struct{}

func (RefreshArgs) Kind() string { return "refresh_repositories" }

type AnalyzeWorker struct {
	river.WorkerDefaults[AnalyzeArgs]
	pool           *pgxpool.Pool
	runner         git.Runner
	github         *GitHubClient
	reposDir       string
	sizeLimitBytes int64
}

type RefreshWorker struct {
	river.WorkerDefaults[RefreshArgs]
	pool *pgxpool.Pool
}

type analysisTarget struct {
	repositoryID     int64
	cloneURL         string
	cloneDir         string
	lastAnalyzedHead *string
}

type analysisResult struct {
	defaultBranch  string
	headSHA        string
	cloneSizeBytes int64
	replaceHistory bool
	newCommits     []git.Commit
	commitOrder    []string
}

func NewClient(pool *pgxpool.Pool, cfg Config) (*river.Client[pgx.Tx], error) {
	githubClient, err := NewGitHubClient(cfg.GitToken)
	if err != nil {
		return nil, fmt.Errorf("create github client: %w", err)
	}

	workers := river.NewWorkers()
	river.AddWorker(workers, NewAnalyzeWorker(pool, githubClient, cfg))
	river.AddWorker(workers, &RefreshWorker{pool: pool})

	refreshJob := river.NewPeriodicJob(
		river.PeriodicInterval(refreshInterval),
		func() (river.JobArgs, *river.InsertOpts) { return RefreshArgs{}, nil },
		nil,
	)

	return river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues:       map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: analysisWorkers}},
		Workers:      workers,
		PeriodicJobs: []*river.PeriodicJob{refreshJob},
	})
}

func NewAnalyzeWorker(pool *pgxpool.Pool, githubClient *GitHubClient, cfg Config) *AnalyzeWorker {
	return &AnalyzeWorker{
		pool:           pool,
		runner:         git.ExecRunner{Token: cfg.GitToken},
		github:         githubClient,
		reposDir:       cfg.ReposDir,
		sizeLimitBytes: cfg.SizeLimitBytes,
	}
}

func ClonePath(reposDir string, repositoryID int64) string {
	return filepath.Join(reposDir, strconv.FormatInt(repositoryID, 10)+".git")
}

// EnqueueAnalysis inserts the job and marks the repository queued; a job already waiting or running is kept
func EnqueueAnalysis(ctx context.Context, pool *pgxpool.Pool, jobs *river.Client[pgx.Tx], repositoryID int64) error {
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		result, err := jobs.InsertTx(ctx, tx, AnalyzeArgs{RepositoryID: repositoryID}, nil)
		if err != nil {
			return fmt.Errorf("insert analysis job: %w", err)
		}

		if result.UniqueSkippedAsDuplicate {
			return nil
		}

		_, err = tx.Exec(ctx,
			"UPDATE repositories SET status = 'queued', status_reason = NULL WHERE id = $1", repositoryID)
		return err
	})
}

func (w *RefreshWorker) Work(ctx context.Context, _ *river.Job[RefreshArgs]) error {
	rows, err := w.pool.Query(ctx, "SELECT id FROM repositories WHERE status IN ('done', 'failed') ORDER BY id")
	if err != nil {
		return err
	}

	repositoryIDs, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return err
	}

	jobs := river.ClientFromContext[pgx.Tx](ctx)
	for _, repositoryID := range repositoryIDs {
		if err := EnqueueAnalysis(ctx, w.pool, jobs, repositoryID); err != nil {
			return fmt.Errorf("enqueue repository %d: %w", repositoryID, err)
		}
	}

	return nil
}

func (w *AnalyzeWorker) Timeout(*river.Job[AnalyzeArgs]) time.Duration {
	return analysisTimeout
}

func (w *AnalyzeWorker) Work(ctx context.Context, job *river.Job[AnalyzeArgs]) error {
	return w.Analyze(ctx, job.Args.RepositoryID)
}

// Analyze stores an analysis failure as the "failed" status with a reason and returns nil,
// so only database errors are returned and retried by river
func (w *AnalyzeWorker) Analyze(ctx context.Context, repositoryID int64) error {
	target := analysisTarget{repositoryID: repositoryID, cloneDir: ClonePath(w.reposDir, repositoryID)}
	err := w.pool.QueryRow(ctx, `
		UPDATE repositories
		SET status = 'running', status_reason = NULL, analysis_started_at = now(), analysis_finished_at = NULL
		WHERE id = $1
		RETURNING clone_url, last_analyzed_head`,
		repositoryID).Scan(&target.cloneURL, &target.lastAnalyzedHead)
	if errors.Is(err, pgx.ErrNoRows) {
		return os.RemoveAll(target.cloneDir)
	}

	if err != nil {
		return fmt.Errorf("mark repository %d running: %w", repositoryID, err)
	}

	hosting := w.github.Metadata(ctx, target.cloneURL)
	analysisErr := w.analyzeAndSave(ctx, target, hosting)
	if analysisErr == nil {
		return nil
	}

	if errors.Is(analysisErr, errRepositoryChanged) {
		return os.RemoveAll(target.cloneDir)
	}
	slog.Warn("repository analysis failed", "repositoryId", repositoryID, "error", analysisErr)

	// A timeout or shutdown cancels ctx, but the failure must still be recorded
	updateCtx := context.WithoutCancel(ctx)
	result, err := w.pool.Exec(updateCtx, `
		UPDATE repositories
		SET status = 'failed', status_reason = $3, hosting_metadata = $4, analysis_finished_at = now()
		WHERE id = $1 AND clone_url = $2`,
		repositoryID, target.cloneURL, analysisErr.Error(), hosting)
	if err != nil {
		return fmt.Errorf("mark repository %d failed: %w", repositoryID, err)
	}

	if result.RowsAffected() == 0 {
		return os.RemoveAll(target.cloneDir)
	}

	return nil
}

func (w *AnalyzeWorker) analyzeAndSave(ctx context.Context, target analysisTarget, hosting *HostingMetadata) error {
	hostingSizeBytes := int64(0)
	if hosting != nil {
		hostingSizeBytes = int64(hosting.SizeKB) * bytesPerKB
	}

	if hostingSizeBytes > w.sizeLimitBytes {
		return w.removeOversizedClone(target.cloneDir)
	}

	if err := w.updateClone(ctx, target); err != nil {
		return err
	}

	cloneSizeBytes, err := git.DirSize(target.cloneDir)
	if err != nil {
		return fmt.Errorf("measure clone size: %w", err)
	}

	if cloneSizeBytes > w.sizeLimitBytes {
		return w.removeOversizedClone(target.cloneDir)
	}

	result, err := w.readHistory(ctx, target)
	if err != nil {
		return fmt.Errorf("read history failed: %w", err)
	}

	result.cloneSizeBytes = cloneSizeBytes

	return w.save(ctx, target, hosting, result)
}

func (w *AnalyzeWorker) removeOversizedClone(cloneDir string) error {
	if err := os.RemoveAll(cloneDir); err != nil {
		return fmt.Errorf("remove oversized clone: %w", err)
	}

	return errSizeLimitExceeded
}

func (w *AnalyzeWorker) updateClone(ctx context.Context, target analysisTarget) error {
	_, statErr := os.Stat(target.cloneDir)
	hasClone := statErr == nil
	hasFinishedAnalysis := target.lastAnalyzedHead != nil
	if hasClone && hasFinishedAnalysis {
		if err := git.Fetch(ctx, w.runner, target.cloneDir); err != nil {
			return fmt.Errorf("fetch failed: %w", err)
		}

		return nil
	}

	// A clone without a finished analysis may be partial or come from an old clone URL
	if err := os.RemoveAll(target.cloneDir); err != nil {
		return fmt.Errorf("remove old clone: %w", err)
	}

	if err := git.CloneBare(ctx, w.runner, target.cloneURL, target.cloneDir); err != nil {
		return fmt.Errorf("clone failed: %w", err)
	}

	return nil
}

func (w *AnalyzeWorker) readHistory(ctx context.Context, target analysisTarget) (analysisResult, error) {
	var result analysisResult
	var err error
	if result.defaultBranch, err = git.DefaultBranch(ctx, w.runner, target.cloneDir); err != nil {
		return analysisResult{}, err
	}

	if result.headSHA, err = git.HeadSHA(ctx, w.runner, target.cloneDir); err != nil {
		return analysisResult{}, err
	}

	if result.headSHA == "" {
		result.replaceHistory = true
		return result, nil
	}

	revRange := result.headSHA
	result.replaceHistory = true
	if target.lastAnalyzedHead != nil {
		isAncestor, err := git.IsAncestor(ctx, w.runner, target.cloneDir, *target.lastAnalyzedHead, result.headSHA)
		// After a force push the old head may be missing from the clone, which also means a rewrite
		isHistoryAppended := err == nil && isAncestor
		if isHistoryAppended {
			revRange = *target.lastAnalyzedHead + ".." + result.headSHA
			result.replaceHistory = false
		}
	}

	if result.newCommits, err = git.Log(ctx, w.runner, target.cloneDir, revRange); err != nil {
		return analysisResult{}, err
	}

	if result.commitOrder, err = git.CommitOrder(ctx, w.runner, target.cloneDir); err != nil {
		return analysisResult{}, err
	}

	return result, nil
}

func (w *AnalyzeWorker) save(ctx context.Context, target analysisTarget, hosting *HostingMetadata, result analysisResult) error {
	err := pgx.BeginFunc(ctx, w.pool, func(tx pgx.Tx) error {
		// Locking the row first makes a concurrent student removal wait for this transaction
		var lockedID int64
		err := tx.QueryRow(ctx,
			"SELECT id FROM repositories WHERE id = $1 AND clone_url = $2 FOR UPDATE",
			target.repositoryID, target.cloneURL).Scan(&lockedID)
		if errors.Is(err, pgx.ErrNoRows) {
			return errRepositoryChanged
		}

		if err != nil {
			return err
		}

		if result.replaceHistory {
			if _, err := tx.Exec(ctx, "DELETE FROM commits WHERE repository_id = $1", target.repositoryID); err != nil {
				return err
			}
		}

		if err := insertCommits(ctx, tx, target.repositoryID, result.newCommits); err != nil {
			return err
		}

		if err := replaceRapidSeries(ctx, tx, target.repositoryID, result.commitOrder); err != nil {
			return err
		}

		var headSHA *string
		if result.headSHA != "" {
			headSHA = &result.headSHA
		}

		_, err = tx.Exec(ctx, `
			UPDATE repositories
			SET status = 'done', status_reason = NULL, analysis_finished_at = now(),
				default_branch = $2, hosting_metadata = $3, last_analyzed_head = $4, clone_size_bytes = $5,
				last_commit_at = (SELECT max(authored_at) FROM commits WHERE repository_id = $1)
			WHERE id = $1`,
			target.repositoryID, result.defaultBranch, hosting, headSHA, result.cloneSizeBytes)

		return err
	})

	if err != nil && !errors.Is(err, errRepositoryChanged) {
		return fmt.Errorf("save results failed: %w", err)
	}

	return err
}

// insertCommits skips rows that already exist, because a commit with the same SHA never changes
func insertCommits(ctx context.Context, tx pgx.Tx, repositoryID int64, commits []git.Commit) error {
	batch := &pgx.Batch{}
	for _, commit := range commits {
		batch.Queue(`
			INSERT INTO commits (repository_id, sha, author_name, author_email, authored_at, committed_at,
				message, is_merge, lines_added, lines_deleted)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			ON CONFLICT DO NOTHING`,
			repositoryID, commit.SHA, commit.AuthorName, commit.AuthorEmail, commit.AuthoredAt, commit.CommittedAt,
			commit.Message, commit.IsMerge, commit.LinesAdded, commit.LinesDeleted)

		for _, file := range commit.Files {
			var oldPath *string
			if file.OldPath != "" {
				oldPath = &file.OldPath
			}

			batch.Queue(`
				INSERT INTO file_changes (repository_id, sha, path, old_path, lines_added, lines_deleted, is_binary)
				VALUES ($1, $2, $3, $4, $5, $6, $7)
				ON CONFLICT DO NOTHING`,
				repositoryID, commit.SHA, file.Path, oldPath, file.LinesAdded, file.LinesDeleted, file.IsBinary)
		}
	}

	return tx.SendBatch(ctx, batch).Close()
}

// replaceRapidSeries recomputes the series over all stored commits, ordered like git log
func replaceRapidSeries(ctx context.Context, tx pgx.Tx, repositoryID int64, commitOrder []string) error {
	if _, err := tx.Exec(ctx, "DELETE FROM rapid_series WHERE repository_id = $1", repositoryID); err != nil {
		return err
	}

	rows, err := tx.Query(ctx, `
		SELECT sha, author_name, author_email, authored_at, committed_at, message, is_merge, lines_added, lines_deleted
		FROM commits
		WHERE repository_id = $1`,
		repositoryID)

	if err != nil {
		return err
	}

	storedCommits, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (git.Commit, error) {
		var commit git.Commit
		err := row.Scan(&commit.SHA, &commit.AuthorName, &commit.AuthorEmail, &commit.AuthoredAt, &commit.CommittedAt,
			&commit.Message, &commit.IsMerge, &commit.LinesAdded, &commit.LinesDeleted)

		return commit, err
	})

	if err != nil {
		return err
	}

	commitsBySHA := make(map[string]git.Commit, len(storedCommits))
	for _, commit := range storedCommits {
		commitsBySHA[commit.SHA] = commit
	}

	orderedCommits := make([]git.Commit, 0, len(storedCommits))
	for _, sha := range commitOrder {
		if commit, found := commitsBySHA[sha]; found {
			orderedCommits = append(orderedCommits, commit)
		}
	}

	batch := &pgx.Batch{}
	for _, series := range metrics.RapidSeries(orderedCommits) {
		seriesSHAs := make([]string, 0, len(series.Commits))
		for _, commit := range series.Commits {
			seriesSHAs = append(seriesSHAs, commit.SHA)
		}

		batch.Queue(
			"INSERT INTO rapid_series (repository_id, commit_shas, started_at, ended_at) VALUES ($1, $2, $3, $4)",
			repositoryID, seriesSHAs, series.StartedAt, series.EndedAt)
	}

	return tx.SendBatch(ctx, batch).Close()
}
