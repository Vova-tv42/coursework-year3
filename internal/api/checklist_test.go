package api

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"coursework/internal/metrics"
)

func TestChecklistOfAnalyzedRepository(t *testing.T) {
	testAPI := newTestAPI(t)
	anna := testAPI.register("anna", roleSupervisor, nil)
	student, _ := testAPI.addAnalyzedStudent(anna, "Alice", "checklist_missing_presentation")

	response := anna.call(http.MethodGet, fmt.Sprintf("/repos/%d/checklist", student.Repository.ID), nil)
	require.Equal(t, http.StatusOK, response.status, string(response.body))
	checklist := decode[checklistResponse](t, response)

	require.True(t, checklist.Evaluated)
	require.False(t, checklist.Admitted)
	require.Equal(t, []metrics.ChecklistItem{
		{Key: metrics.ChecklistCode, Found: true, Files: []string{"src/main.go"}},
		{Key: metrics.ChecklistExplanatoryNote, Found: true, Files: []string{"docs/Пояснювальна записка.pdf"}},
		{Key: metrics.ChecklistPresentation, Found: false, Files: []string{}},
		{Key: metrics.ChecklistReadmeDeployment, Found: true, Files: []string{"README.md"}},
	}, checklist.Items)
	require.Equal(t, 4, testAPI.countRows("snapshot_files", student.Repository.ID))
}

func TestChecklistBeforeAnalysisIsNotEvaluated(t *testing.T) {
	testAPI := newTestAPI(t)
	anna := testAPI.register("anna", roleSupervisor, nil)
	student := anna.addStudent("Alice", "bachelor")

	response := anna.call(http.MethodGet, fmt.Sprintf("/repos/%d/checklist", student.Repository.ID), nil)
	require.Equal(t, http.StatusOK, response.status, string(response.body))
	require.JSONEq(t, `{"evaluated":false,"admitted":false,"items":[]}`, string(response.body))
}

func TestChecklistOfForeignRepositoryIsNotFound(t *testing.T) {
	testAPI := newTestAPI(t)
	anna := testAPI.register("anna", roleSupervisor, nil)
	boris := testAPI.register("boris", roleSupervisor, nil)
	student, _ := testAPI.addAnalyzedStudent(anna, "Alice", "checklist_complete")

	response := boris.call(http.MethodGet, fmt.Sprintf("/repos/%d/checklist", student.Repository.ID), nil)
	requireError(t, response, http.StatusNotFound, "not_found")
}

func TestStudentsAdmittedFlagAndFilter(t *testing.T) {
	testAPI := newTestAPI(t)
	anna := testAPI.register("anna", roleSupervisor, nil)
	manager := testAPI.register("manager", roleRepoManager, nil)
	testAPI.addAnalyzedStudent(anna, "Alice", "checklist_complete")
	testAPI.addAnalyzedStudent(anna, "Bob", "checklist_readme_no_deploy")
	anna.addStudent("Carol", "bachelor")

	response := manager.call(http.MethodGet, "/students", nil)
	require.Equal(t, http.StatusOK, response.status, string(response.body))
	admittedByName := map[string]*bool{}
	for _, student := range decode[[]studentOverview](t, response) {
		admittedByName[student.FullName] = student.Flags.Admitted
	}

	isAdmitted, isNotAdmitted := true, false
	require.Equal(t, map[string]*bool{"Alice": &isAdmitted, "Bob": &isNotAdmitted, "Carol": nil}, admittedByName)
	require.Contains(t, string(response.body), `"admitted":null`)

	require.Equal(t, []string{"Alice"}, manager.listStudentNames("/students?admitted=true"))
	require.Equal(t, []string{"Bob"}, manager.listStudentNames("/students?admitted=false"))
	require.Equal(t, []string{}, manager.listStudentNames("/students?admitted=false&rapidSeries=true"))
	requireError(t, manager.call(http.MethodGet, "/students?admitted=yes", nil), http.StatusBadRequest, "invalid_input")
}
