package worker

import (
	"context"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"coursework/internal/fixture"
	"coursework/internal/git"
	"coursework/internal/metrics"
	"coursework/internal/testdb"
)

const noSizeLimit = 1 << 40

var testDatabase *testdb.Database

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	database, err := testdb.Start(context.Background())
	defer database.Terminate()
	if err != nil {
		log.Printf("start test database: %v", err)
		return 1
	}
	testDatabase = database
	return m.Run()
}

// recordingRunner remembers the revision range of every git log call
type recordingRunner struct {
	git.ExecRunner
	logRanges []string
}

func (r *recordingRunner) Run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	if slices.Contains(args, "log") {
		// Log passes the range right before the final "--"
		r.logRanges = append(r.logRanges, args[len(args)-2])
	}

	return r.ExecRunner.Run(ctx, dir, args...)
}

type workerTest struct {
	t            *testing.T
	pool         *pgxpool.Pool
	worker       *AnalyzeWorker
	runner       *recordingRunner
	sourceDir    string
	repositoryID int64
}

func newWorkerTest(t *testing.T, scenario string, sizeLimitBytes int64) *workerTest {
	t.Helper()
	pool := testDatabase.NewPool(t)
	sourceDir := filepath.Join(t.TempDir(), scenario)
	require.NoError(t, fixture.Build(scenario, sourceDir))

	githubClient, err := NewGitHubClient("")
	require.NoError(t, err)
	worker := NewAnalyzeWorker(pool, githubClient, Config{ReposDir: t.TempDir(), SizeLimitBytes: sizeLimitBytes})
	runner := &recordingRunner{}
	worker.runner = runner

	test := &workerTest{t: t, pool: pool, worker: worker, runner: runner, sourceDir: sourceDir}
	test.repositoryID = test.addRepository(sourceDir)
	return test
}

func (test *workerTest) addRepository(cloneURL string) int64 {
	test.t.Helper()
	var repositoryID int64
	err := test.pool.QueryRow(test.t.Context(), `
		WITH supervisor AS (
			INSERT INTO users (email, password_hash, display_name, role)
			VALUES ('anna@example.com', 'hash', 'Anna', 'supervisor') RETURNING id
		), student AS (
			INSERT INTO students (supervisor_id, full_name, stream) SELECT id, 'Alice', 'bachelor' FROM supervisor RETURNING id
		)
		INSERT INTO repositories (student_id, clone_url) SELECT id, $1 FROM student RETURNING id`,
		cloneURL).Scan(&repositoryID)

	require.NoError(test.t, err)
	return repositoryID
}

func (test *workerTest) analyze() {
	test.t.Helper()
	test.runner.logRanges = nil
	require.NoError(test.t, test.worker.Analyze(test.t.Context(), test.repositoryID))
}

type repositoryState struct {
	Status           string
	StatusReason     *string
	DefaultBranch    *string
	LastAnalyzedHead *string
	CloneSizeBytes   *int64
}

func (test *workerTest) repositoryState() repositoryState {
	test.t.Helper()
	var state repositoryState
	err := test.pool.QueryRow(test.t.Context(), `
		SELECT status, status_reason, default_branch, last_analyzed_head, clone_size_bytes
		FROM repositories WHERE id = $1`,
		test.repositoryID).Scan(&state.Status, &state.StatusReason, &state.DefaultBranch, &state.LastAnalyzedHead, &state.CloneSizeBytes)

	require.NoError(test.t, err)
	return state
}

// storedRows returns every row of the repository's commits, file changes and rapid series as JSON
func (test *workerTest) storedRows() []string {
	test.t.Helper()
	rows, err := test.pool.Query(test.t.Context(), `
		SELECT to_jsonb(c)::text FROM commits c WHERE repository_id = $1
		UNION ALL SELECT to_jsonb(f)::text FROM file_changes f WHERE repository_id = $1
		UNION ALL SELECT (to_jsonb(r) - 'id')::text FROM rapid_series r WHERE repository_id = $1
		ORDER BY 1`,
		test.repositoryID)

	require.NoError(test.t, err)
	storedRows, err := pgx.CollectRows(rows, pgx.RowTo[string])
	require.NoError(test.t, err)

	return storedRows
}

func (test *workerTest) storedSHAs() []string {
	test.t.Helper()
	rows, err := test.pool.Query(test.t.Context(),
		"SELECT sha FROM commits WHERE repository_id = $1 ORDER BY sha", test.repositoryID)

	require.NoError(test.t, err)
	shas, err := pgx.CollectRows(rows, pgx.RowTo[string])
	require.NoError(test.t, err)

	return shas
}

func (test *workerTest) sourceCommits() []git.Commit {
	test.t.Helper()
	commits, err := git.Log(test.t.Context(), git.ExecRunner{}, test.sourceDir, "HEAD")
	require.NoError(test.t, err)

	return commits
}

func (test *workerTest) sourceSHAs() []string {
	test.t.Helper()
	var shas []string
	for _, commit := range test.sourceCommits() {
		shas = append(shas, commit.SHA)
	}

	slices.Sort(shas)
	return shas
}

func (test *workerTest) sourceHead() string {
	test.t.Helper()
	headSHA, err := git.HeadSHA(test.t.Context(), git.ExecRunner{}, test.sourceDir)
	require.NoError(test.t, err)

	return headSHA
}

func (test *workerTest) gitInSource(args ...string) {
	test.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = test.sourceDir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=Ivan Petrenko", "GIT_AUTHOR_EMAIL=ivan.petrenko@example.com",
		"GIT_COMMITTER_NAME=Ivan Petrenko", "GIT_COMMITTER_EMAIL=ivan.petrenko@example.com")

	output, err := cmd.CombinedOutput()
	require.NoError(test.t, err, string(output))
}

func (test *workerTest) commitInSource(fileName, content string) {
	test.t.Helper()
	require.NoError(test.t, os.WriteFile(filepath.Join(test.sourceDir, fileName), []byte(content), 0o644))
	test.gitInSource("add", fileName)
	test.gitInSource("commit", "--quiet", "-m", "Add "+fileName)
}

func TestFullAnalysisStoresHistoryAndRapidSeries(t *testing.T) {
	test := newWorkerTest(t, "rapid", noSizeLimit)
	test.analyze()

	state := test.repositoryState()
	require.Equal(t, "done", state.Status)
	require.Nil(t, state.StatusReason)
	require.Equal(t, "main", *state.DefaultBranch)
	require.Equal(t, test.sourceHead(), *state.LastAnalyzedHead)
	require.Positive(t, *state.CloneSizeBytes)
	require.Equal(t, []string{test.sourceHead()}, test.runner.logRanges)
	require.Equal(t, test.sourceSHAs(), test.storedSHAs())
	require.DirExists(t, ClonePath(test.worker.reposDir, test.repositoryID))

	expectedFileChanges := 0
	for _, commit := range test.sourceCommits() {
		expectedFileChanges += len(commit.Files)
	}
	var storedFileChanges int
	require.NoError(t, test.pool.QueryRow(t.Context(),
		"SELECT count(*) FROM file_changes WHERE repository_id = $1", test.repositoryID).Scan(&storedFileChanges))
	require.Equal(t, expectedFileChanges, storedFileChanges)

	expectedSeries := metrics.RapidSeries(test.sourceCommits())
	require.Len(t, expectedSeries, 1)
	var storedSHAs []string
	require.NoError(t, test.pool.QueryRow(t.Context(),
		"SELECT commit_shas FROM rapid_series WHERE repository_id = $1", test.repositoryID).Scan(&storedSHAs))
	var expectedSHAs []string

	for _, commit := range expectedSeries[0].Commits {
		expectedSHAs = append(expectedSHAs, commit.SHA)
	}

	require.Equal(t, expectedSHAs, storedSHAs)
}

func TestReanalysisGivesIdenticalRows(t *testing.T) {
	test := newWorkerTest(t, "merge_rename", noSizeLimit)
	test.analyze()
	firstRows := test.storedRows()
	require.NotEmpty(t, firstRows)

	test.analyze()
	require.Equal(t, firstRows, test.storedRows())
	require.Equal(t, "done", test.repositoryState().Status)
}

func TestNewCommitsAreProcessedIncrementally(t *testing.T) {
	test := newWorkerTest(t, "sizes", noSizeLimit)
	test.analyze()
	oldHead := test.sourceHead()

	test.commitInSource("notes.txt", "one\ntwo\n")
	test.analyze()

	newHead := test.sourceHead()
	require.Equal(t, []string{oldHead + ".." + newHead}, test.runner.logRanges)
	require.Equal(t, test.sourceSHAs(), test.storedSHAs())
	require.Len(t, test.storedSHAs(), 42)
	require.Equal(t, newHead, *test.repositoryState().LastAnalyzedHead)
}

func TestRewrittenHistoryIsReplaced(t *testing.T) {
	test := newWorkerTest(t, "sizes", noSizeLimit)
	test.analyze()
	removedSHAs := test.storedSHAs()

	test.gitInSource("reset", "--hard", "--quiet", "HEAD~3")
	test.commitInSource("rewritten.txt", "after force push\n")
	test.analyze()

	require.Equal(t, []string{test.sourceHead()}, test.runner.logRanges)
	require.Equal(t, test.sourceSHAs(), test.storedSHAs())
	require.Len(t, test.storedSHAs(), 39)
	require.NotSubset(t, test.storedSHAs(), removedSHAs)
}

func TestEmptyRepositoryIsDone(t *testing.T) {
	test := newWorkerTest(t, "empty", noSizeLimit)
	test.analyze()

	state := test.repositoryState()
	require.Equal(t, "done", state.Status)
	require.Nil(t, state.LastAnalyzedHead)
	require.Empty(t, test.storedSHAs())
}

func TestSizeLimitFailsAndRemovesClone(t *testing.T) {
	test := newWorkerTest(t, "sizes", 1)
	test.analyze()

	state := test.repositoryState()
	require.Equal(t, "failed", state.Status)
	require.Equal(t, "size limit exceeded", *state.StatusReason)
	require.NoDirExists(t, ClonePath(test.worker.reposDir, test.repositoryID))
	require.Empty(t, test.storedSHAs())
}

func TestInvalidCloneURLFailsWithReason(t *testing.T) {
	test := newWorkerTest(t, "empty", noSizeLimit)
	_, err := test.pool.Exec(t.Context(), "UPDATE repositories SET clone_url = $2 WHERE id = $1",
		test.repositoryID, filepath.Join(t.TempDir(), "missing"))
	require.NoError(t, err)
	test.analyze()

	state := test.repositoryState()
	require.Equal(t, "failed", state.Status)
	require.Contains(t, *state.StatusReason, "clone failed: ")
}

func TestFailedRefreshKeepsPreviousData(t *testing.T) {
	test := newWorkerTest(t, "rapid", noSizeLimit)
	test.analyze()
	analyzedRows := test.storedRows()

	require.NoError(t, os.RemoveAll(test.sourceDir))
	test.analyze()

	state := test.repositoryState()
	require.Equal(t, "failed", state.Status)
	require.Contains(t, *state.StatusReason, "fetch failed: ")
	require.Equal(t, analyzedRows, test.storedRows())
}

func TestRemovedRepositoryIsSkipped(t *testing.T) {
	test := newWorkerTest(t, "rapid", noSizeLimit)
	_, err := test.pool.Exec(t.Context(), "DELETE FROM repositories WHERE id = $1", test.repositoryID)
	require.NoError(t, err)

	test.analyze()
	require.NoDirExists(t, ClonePath(test.worker.reposDir, test.repositoryID))
}
