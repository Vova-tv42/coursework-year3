package api

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"coursework/internal/metrics"
)

func callSetting[T any](client *apiClient, method, name string, body any) settingResponse[T] {
	client.t.Helper()
	response := client.call(method, "/settings/"+name, body)
	require.Equal(client.t, http.StatusOK, response.status, string(response.body))
	return decode[settingResponse[T]](client.t, response)
}

func (client *apiClient) commitSizes(repositoryID int64) metrics.CommitSizeStats {
	client.t.Helper()
	response := client.call(http.MethodGet, fmt.Sprintf("/repos/%d/commit-sizes", repositoryID), nil)
	require.Equal(client.t, http.StatusOK, response.status, string(response.body))
	return decode[metrics.CommitSizeStats](client.t, response)
}

func (client *apiClient) rapidSeriesCount(repositoryID int64) int {
	client.t.Helper()
	response := client.call(http.MethodGet, fmt.Sprintf("/repos/%d/activity", repositoryID), nil)
	require.Equal(client.t, http.StatusOK, response.status, string(response.body))
	return len(decode[activityResponse](client.t, response).RapidSeries)
}

func TestOnlyCoordinatorsAndManagersChangeSettings(t *testing.T) {
	testAPI := newTestAPI(t)
	anna := testAPI.register("anna", roleSupervisor, nil)
	coordinator := testAPI.register("coordinator", roleCoordinator, streamOf("bachelor"))

	for _, name := range []string{"junk-patterns", "commit-sizes", "rapid-series"} {
		t.Run(name, func(t *testing.T) {
			defaults := anna.call(http.MethodGet, "/settings/"+name, nil)
			require.Equal(t, http.StatusOK, defaults.status, string(defaults.body))

			requireError(t, anna.call(http.MethodPut, "/settings/"+name, map[string]any{}), http.StatusForbidden, "forbidden")
			requireError(t, anna.call(http.MethodDelete, "/settings/"+name, nil), http.StatusForbidden, "forbidden")

			reset := coordinator.call(http.MethodDelete, "/settings/"+name, nil)
			require.Equal(t, http.StatusOK, reset.status, string(reset.body))
		})
	}
}

func TestCommitSizeRangesAreSortedAndApplied(t *testing.T) {
	testAPI := newTestAPI(t)
	anna := testAPI.register("anna", roleSupervisor, nil)
	manager := testAPI.register("manager", roleRepoManager, nil)
	student, _ := testAPI.addAnalyzedStudent(anna, "Alice", "sizes")

	saved := callSetting[metrics.CommitSizeSettings](manager, http.MethodPut, "commit-sizes", metrics.CommitSizeSettings{
		Ranges:           []metrics.CommitSizeRange{{From: 301}, {From: 0, To: new(300)}},
		LargeCommitLines: new(250),
	})
	require.Equal(t, []metrics.CommitSizeRange{{From: 0, To: new(300)}, {From: 301}}, saved.Value.Ranges)
	require.False(t, saved.IsDefault)

	sizes := anna.commitSizes(student.Repository.ID)
	require.Equal(t, []metrics.SizeBucket{{Label: "0-300", Count: 40}, {Label: "301+", Count: 1}}, sizes.Buckets)
	require.Equal(t, metrics.CommitSizeRange{From: 251}, sizes.LargeCommitRange)
	require.Len(t, sizes.LargeCommits, 1)
}

func TestInvalidCommitSizeSettingsAreRejected(t *testing.T) {
	testAPI := newTestAPI(t)
	manager := testAPI.register("manager", roleRepoManager, nil)

	invalidSettings := map[string]metrics.CommitSizeSettings{
		"no ranges":          {Ranges: []metrics.CommitSizeRange{}},
		"two open ranges":    {Ranges: []metrics.CommitSizeRange{{From: 0, To: new(10)}, {From: 11}, {From: 500}}},
		"overlapping ranges": {Ranges: []metrics.CommitSizeRange{{From: 0, To: new(100)}, {From: 50, To: new(200)}}},
		"end before start":   {Ranges: []metrics.CommitSizeRange{{From: 100, To: new(10)}}},
		"negative start":     {Ranges: []metrics.CommitSizeRange{{From: -1, To: new(10)}}},
		"negative large":     {Ranges: []metrics.CommitSizeRange{{From: 0}}, LargeCommitLines: new(-1)},
	}
	for name, settings := range invalidSettings {
		t.Run(name, func(t *testing.T) {
			requireError(t, manager.call(http.MethodPut, "/settings/commit-sizes", settings), http.StatusBadRequest, "invalid_input")
		})
	}

	require.True(t, callSetting[metrics.CommitSizeSettings](manager, http.MethodGet, "commit-sizes", nil).IsDefault)
}

func TestRapidSeriesSettingsRecomputeStoredSeries(t *testing.T) {
	testAPI := newTestAPI(t)
	anna := testAPI.register("anna", roleSupervisor, nil)
	manager := testAPI.register("manager", roleRepoManager, nil)
	student, _ := testAPI.addAnalyzedStudent(anna, "Alice", "rapid")
	repositoryID := student.Repository.ID
	require.Equal(t, 1, anna.rapidSeriesCount(repositoryID))

	windowOnly := metrics.RapidSeriesSettings{
		Consecutive: metrics.ConsecutiveRule{Enabled: false, MinCommits: 3, MaxGapSeconds: 60},
		Window:      metrics.WindowRule{Enabled: true, MinCommits: 2, WindowSeconds: 30},
	}
	callSetting[metrics.RapidSeriesSettings](manager, http.MethodPut, "rapid-series", windowOnly)
	require.Equal(t, 2, anna.rapidSeriesCount(repositoryID))

	tooManyCommits := metrics.DefaultRapidSeriesSettings
	tooManyCommits.Consecutive.MinCommits = 5
	callSetting[metrics.RapidSeriesSettings](manager, http.MethodPut, "rapid-series", tooManyCommits)
	require.Zero(t, anna.rapidSeriesCount(repositoryID))
	require.Equal(t, []string{}, anna.listStudentNames("/students?rapidSeries=true"))

	callSetting[metrics.RapidSeriesSettings](manager, http.MethodDelete, "rapid-series", nil)
	require.Equal(t, 1, anna.rapidSeriesCount(repositoryID))
	require.Equal(t, []string{"Alice"}, anna.listStudentNames("/students?rapidSeries=true"))
}

func TestInvalidRapidSeriesSettingsAreRejected(t *testing.T) {
	testAPI := newTestAPI(t)
	manager := testAPI.register("manager", roleRepoManager, nil)

	noRuleEnabled := metrics.DefaultRapidSeriesSettings
	noRuleEnabled.Consecutive.Enabled = false
	singleCommit := metrics.DefaultRapidSeriesSettings
	singleCommit.Consecutive.MinCommits = 1
	zeroWindow := metrics.DefaultRapidSeriesSettings
	zeroWindow.Window.WindowSeconds = 0

	invalidSettings := map[string]metrics.RapidSeriesSettings{
		"no rule enabled": noRuleEnabled,
		"single commit":   singleCommit,
		"zero window":     zeroWindow,
	}
	for name, settings := range invalidSettings {
		t.Run(name, func(t *testing.T) {
			requireError(t, manager.call(http.MethodPut, "/settings/rapid-series", settings), http.StatusBadRequest, "invalid_input")
		})
	}

	require.True(t, callSetting[metrics.RapidSeriesSettings](manager, http.MethodGet, "rapid-series", nil).IsDefault)
}
