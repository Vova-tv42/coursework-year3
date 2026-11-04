package api

import (
	"fmt"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"coursework/internal/fixture"
	"coursework/internal/git"
	"coursework/internal/metrics"
	"coursework/internal/worker"
)

const noSizeLimit = 1 << 40

func TestActivityMatchesMetrics(t *testing.T) {
	testAPI := newTestAPI(t)
	anna := testAPI.register("anna", roleSupervisor, nil)
	student, sourceCommits := testAPI.addAnalyzedStudent(anna, "Alice", "rapid")

	response := anna.call(http.MethodGet, fmt.Sprintf("/repos/%d/activity", student.Repository.ID), nil)
	require.Equal(t, http.StatusOK, response.status, string(response.body))
	activity := decode[activityResponse](t, response)

	require.Equal(t, metrics.TimeZone, activity.TimeZone)
	require.Equal(t, metrics.DailyActivity(sourceCommits), activity.Days)
	require.Len(t, activity.RapidSeries, 1)
	require.Equal(t, rapidSeriesInUTC(metrics.RapidSeries(sourceCommits)), rapidSeriesInUTC(activity.RapidSeries))
}

func TestCommitSizesMatchMetrics(t *testing.T) {
	testAPI := newTestAPI(t)
	anna := testAPI.register("anna", roleSupervisor, nil)
	student, sourceCommits := testAPI.addAnalyzedStudent(anna, "Alice", "sizes")

	response := anna.call(http.MethodGet, fmt.Sprintf("/repos/%d/commit-sizes", student.Repository.ID), nil)
	require.Equal(t, http.StatusOK, response.status, string(response.body))
	sizes := decode[metrics.CommitSizeStats](t, response)

	expected := metrics.CommitSizes(sourceCommits)
	require.Len(t, sizes.LargeCommits, 1)
	expected.LargeCommits = summariesInUTC(expected.LargeCommits)
	sizes.LargeCommits = summariesInUTC(sizes.LargeCommits)
	require.Equal(t, expected, sizes)
}

func TestNeverAnalyzedRepositoryHasEmptyResults(t *testing.T) {
	testAPI := newTestAPI(t)
	anna := testAPI.register("anna", roleSupervisor, nil)
	student := anna.addStudent("Alice", "bachelor")
	repositoryPath := fmt.Sprintf("/repos/%d", student.Repository.ID)

	response := anna.call(http.MethodGet, repositoryPath+"/activity", nil)
	require.Equal(t, http.StatusOK, response.status, string(response.body))
	require.JSONEq(t, `{"timeZone":"Europe/Kyiv","days":[],"rapidSeries":[]}`, string(response.body))

	response = anna.call(http.MethodGet, repositoryPath+"/commit-sizes", nil)
	require.Equal(t, http.StatusOK, response.status, string(response.body))
	sizes := decode[metrics.CommitSizeStats](t, response)
	require.Zero(t, sizes.NonMergeCommits)
	require.Len(t, sizes.Buckets, 5)
	require.Contains(t, string(response.body), `"largeCommits":[]`)
}

func TestActivityOfForeignRepositoryIsNotFound(t *testing.T) {
	testAPI := newTestAPI(t)
	anna := testAPI.register("anna", roleSupervisor, nil)
	boris := testAPI.register("boris", roleSupervisor, nil)
	masterCoordinator := testAPI.register("masters", roleCoordinator, streamOf("master"))
	student, _ := testAPI.addAnalyzedStudent(anna, "Alice", "rapid")
	repositoryPath := fmt.Sprintf("/repos/%d", student.Repository.ID)

	for _, outsider := range []*apiClient{boris, masterCoordinator} {
		requireError(t, outsider.call(http.MethodGet, repositoryPath+"/activity", nil), http.StatusNotFound, "not_found")
		requireError(t, outsider.call(http.MethodGet, repositoryPath+"/commit-sizes", nil), http.StatusNotFound, "not_found")
	}

	requireError(t, anna.call(http.MethodGet, "/repos/abc/activity", nil), http.StatusNotFound, "not_found")
}

func TestStudentsRapidSeriesFlagAndFilter(t *testing.T) {
	testAPI := newTestAPI(t)
	anna := testAPI.register("anna", roleSupervisor, nil)
	testAPI.addAnalyzedStudent(anna, "Alice", "rapid")
	testAPI.addAnalyzedStudent(anna, "Bob", "sizes")
	anna.addStudent("Carol", "bachelor")

	response := anna.call(http.MethodGet, "/students", nil)
	require.Equal(t, http.StatusOK, response.status, string(response.body))
	rapidSeriesByName := map[string]bool{}
	for _, student := range decode[[]studentOverview](t, response) {
		rapidSeriesByName[student.FullName] = student.Flags.RapidSeries
	}

	require.Equal(t, map[string]bool{"Alice": true, "Bob": false, "Carol": false}, rapidSeriesByName)

	require.Equal(t, []string{"Alice"}, anna.listStudentNames("/students?rapidSeries=true"))
	require.Equal(t, []string{"Bob", "Carol"}, anna.listStudentNames("/students?rapidSeries=false"))
	require.Equal(t, []string{}, anna.listStudentNames("/students?rapidSeries=true&stream=master"))
	requireError(t, anna.call(http.MethodGet, "/students?rapidSeries=maybe", nil), http.StatusBadRequest, "invalid_input")
}

// addAnalyzedStudent points the student's repository at a local fixture, runs the worker and returns the fixture history
func (testAPI *testAPI) addAnalyzedStudent(supervisor *apiClient, fullName, scenario string) (studentOverview, []git.Commit) {
	testAPI.t.Helper()
	t := testAPI.t
	student := supervisor.addStudent(fullName, "bachelor")

	sourceDir := filepath.Join(t.TempDir(), scenario)
	require.NoError(t, fixture.Build(scenario, sourceDir))
	_, err := testAPI.pool.Exec(t.Context(),
		"UPDATE repositories SET clone_url = $1 WHERE id = $2", sourceDir, student.Repository.ID)
	require.NoError(t, err)

	githubClient, err := worker.NewGitHubClient("")
	require.NoError(t, err)
	analyzeWorker := worker.NewAnalyzeWorker(testAPI.pool, githubClient,
		worker.Config{ReposDir: testAPI.reposDir, SizeLimitBytes: noSizeLimit})
	require.NoError(t, analyzeWorker.Analyze(t.Context(), student.Repository.ID))

	sourceCommits, err := git.Log(t.Context(), git.ExecRunner{}, sourceDir, "HEAD")
	require.NoError(t, err)
	return student, sourceCommits
}

// summariesInUTC drops the time zone offsets, which differ between git output and the database
func summariesInUTC(summaries []metrics.CommitSummary) []metrics.CommitSummary {
	converted := make([]metrics.CommitSummary, 0, len(summaries))
	for _, summary := range summaries {
		summary.AuthoredAt = summary.AuthoredAt.UTC()
		converted = append(converted, summary)
	}

	return converted
}

func rapidSeriesInUTC(rapidSeries []metrics.RapidCommitSeries) []metrics.RapidCommitSeries {
	converted := make([]metrics.RapidCommitSeries, 0, len(rapidSeries))
	for _, series := range rapidSeries {
		series.StartedAt = series.StartedAt.UTC()
		series.EndedAt = series.EndedAt.UTC()
		series.Commits = summariesInUTC(series.Commits)
		converted = append(converted, series)
	}

	return converted
}
