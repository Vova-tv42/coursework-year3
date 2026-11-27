package api

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"coursework/internal/metrics"
)

func (client *apiClient) codeStats(repositoryID int64) metrics.CodeStatistics {
	client.t.Helper()
	response := client.call(http.MethodGet, fmt.Sprintf("/repos/%d/code-stats", repositoryID), nil)
	require.Equal(client.t, http.StatusOK, response.status, string(response.body))
	return decode[metrics.CodeStatistics](client.t, response)
}

func TestCodeStatsExcludeDefaultJunk(t *testing.T) {
	testAPI := newTestAPI(t)
	anna := testAPI.register("anna", roleSupervisor, nil)
	student, _ := testAPI.addAnalyzedStudent(anna, "Alice", "junk")

	stats := anna.codeStats(student.Repository.ID)

	require.Equal(t, metrics.LineCounts{Code: 10, Docs: 1, Total: 11}, stats.Lines)
	require.Equal(t, 4, stats.Junk.Files)
	require.Equal(t, 73, stats.Junk.Lines)
	require.Equal(t, []string{
		"assets/logo.png",
		"node_modules/left-pad/index.js",
		"node_modules/left-pad/package.json",
		"vendor/github.com/pkg/errors/errors.go",
	}, stats.Junk.SamplePaths)
}

func TestCodeStatsSplitComments(t *testing.T) {
	testAPI := newTestAPI(t)
	anna := testAPI.register("anna", roleSupervisor, nil)
	student, _ := testAPI.addAnalyzedStudent(anna, "Alice", "comments")

	require.Equal(t, metrics.LineCounts{Code: 25, Comments: 13, Total: 38}, anna.codeStats(student.Repository.ID).Lines)
}

func TestCodeStatsBeforeAnalysisAreEmpty(t *testing.T) {
	testAPI := newTestAPI(t)
	anna := testAPI.register("anna", roleSupervisor, nil)
	student := anna.addStudent("Alice", "bachelor")

	response := anna.call(http.MethodGet, fmt.Sprintf("/repos/%d/code-stats", student.Repository.ID), nil)
	require.Equal(t, http.StatusOK, response.status, string(response.body))
	require.JSONEq(t, `{
		"lines": {"code": 0, "comments": 0, "docs": 0, "tests": 0, "config": 0, "total": 0},
		"junk": {"files": 0, "bytes": 0, "lines": 0, "samplePaths": []}
	}`, string(response.body))
}

func TestCodeStatsOfForeignRepositoryIsNotFound(t *testing.T) {
	testAPI := newTestAPI(t)
	anna := testAPI.register("anna", roleSupervisor, nil)
	boris := testAPI.register("boris", roleSupervisor, nil)
	student, _ := testAPI.addAnalyzedStudent(anna, "Alice", "junk")

	response := boris.call(http.MethodGet, fmt.Sprintf("/repos/%d/code-stats", student.Repository.ID), nil)
	requireError(t, response, http.StatusNotFound, "not_found")
}

func TestJunkPatternsApplyToEveryone(t *testing.T) {
	testAPI := newTestAPI(t)
	anna := testAPI.register("anna", roleSupervisor, nil)
	manager := testAPI.register("manager", roleRepoManager, nil)
	student, _ := testAPI.addAnalyzedStudent(anna, "Alice", "junk")
	repositoryID := student.Repository.ID
	defaultStats := anna.codeStats(repositoryID)

	customPatterns := junkPatternsSetting{Patterns: []string{"**/*.md", metrics.BinaryPattern}}
	saved := callSetting[junkPatternsSetting](manager, http.MethodPut, "junk-patterns", customPatterns)
	require.Equal(t, settingResponse[junkPatternsSetting]{Value: customPatterns}, saved)
	require.Equal(t, saved, callSetting[junkPatternsSetting](anna, http.MethodGet, "junk-patterns", nil))

	customStats := anna.codeStats(repositoryID)
	require.Equal(t, metrics.LineCounts{Code: 80, Config: 3, Total: 83}, customStats.Lines)
	require.Equal(t, []string{"README.md", "assets/logo.png"}, customStats.Junk.SamplePaths)

	reset := callSetting[junkPatternsSetting](manager, http.MethodDelete, "junk-patterns", nil)
	require.Equal(t, settingResponse[junkPatternsSetting]{Value: defaultJunkPatternsSetting, IsDefault: true}, reset)
	require.Equal(t, defaultStats, anna.codeStats(repositoryID))
}

func TestEmptyJunkPatternListIsSaved(t *testing.T) {
	testAPI := newTestAPI(t)
	manager := testAPI.register("manager", roleRepoManager, nil)

	defaults := callSetting[junkPatternsSetting](manager, http.MethodGet, "junk-patterns", nil)
	require.Equal(t, settingResponse[junkPatternsSetting]{Value: defaultJunkPatternsSetting, IsDefault: true}, defaults)

	emptyList := callSetting[junkPatternsSetting](manager, http.MethodPut, "junk-patterns", junkPatternsSetting{Patterns: []string{}})
	require.Equal(t, settingResponse[junkPatternsSetting]{Value: junkPatternsSetting{Patterns: []string{}}}, emptyList)
	require.Equal(t, emptyList, callSetting[junkPatternsSetting](manager, http.MethodGet, "junk-patterns", nil))
}

func TestInvalidJunkPatternsAreRejected(t *testing.T) {
	testAPI := newTestAPI(t)
	manager := testAPI.register("manager", roleRepoManager, nil)

	invalidBodies := map[string]any{
		"missing list":  map[string]any{},
		"null list":     map[string]any{"patterns": nil},
		"empty pattern": junkPatternsSetting{Patterns: []string{"**/vendor/**", ""}},
		"blank pattern": junkPatternsSetting{Patterns: []string{"  "}},
		"invalid glob":  junkPatternsSetting{Patterns: []string{"src/[abc"}},
	}
	for name, body := range invalidBodies {
		t.Run(name, func(t *testing.T) {
			requireError(t, manager.call(http.MethodPut, "/settings/junk-patterns", body), http.StatusBadRequest, "invalid_input")
		})
	}

	require.True(t, callSetting[junkPatternsSetting](manager, http.MethodGet, "junk-patterns", nil).IsDefault)
}
