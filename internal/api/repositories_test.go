package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"coursework/internal/worker"
)

func TestGetRepositoryIsScoped(t *testing.T) {
	testAPI := newTestAPI(t)
	anna := testAPI.register("anna", roleSupervisor, nil)
	boris := testAPI.register("boris", roleSupervisor, nil)
	repoManager := testAPI.register("manager", roleRepoManager, nil)
	student := anna.addStudent("Alice", "bachelor")
	repositoryPath := fmt.Sprintf("/repos/%d", student.Repository.ID)

	response := anna.call(http.MethodGet, repositoryPath, nil)
	require.Equal(t, http.StatusOK, response.status, string(response.body))
	require.Equal(t, student.Repository, decode[repository](t, response))
	require.Contains(t, string(response.body), `"hosting":null`)

	require.Equal(t, http.StatusOK, repoManager.call(http.MethodGet, repositoryPath, nil).status)
	requireError(t, boris.call(http.MethodGet, repositoryPath, nil), http.StatusNotFound, "not_found")
	requireError(t, boris.call(http.MethodPost, repositoryPath+"/analyze", nil), http.StatusNotFound, "not_found")
	requireError(t, anna.call(http.MethodGet, "/repos/abc", nil), http.StatusNotFound, "not_found")
	require.Zero(t, testAPI.countAnalysisJobs())
}

func TestAnalyzeQueuesOneJobPerRepository(t *testing.T) {
	testAPI := newTestAPI(t)
	anna := testAPI.register("anna", roleSupervisor, nil)
	bachelorCoordinator := testAPI.register("bachelors", roleCoordinator, streamOf("bachelor"))
	student := anna.addStudent("Alice", "bachelor")
	analyzePath := fmt.Sprintf("/repos/%d/analyze", student.Repository.ID)

	response := anna.call(http.MethodPost, analyzePath, nil)
	require.Equal(t, http.StatusAccepted, response.status, string(response.body))
	require.Equal(t, "queued", decode[repository](t, response).Status)

	// Any user in scope may start an analysis; a waiting job is not duplicated
	response = bachelorCoordinator.call(http.MethodPost, analyzePath, nil)
	require.Equal(t, http.StatusAccepted, response.status, string(response.body))
	require.Equal(t, "queued", decode[repository](t, response).Status)
	require.Equal(t, 1, testAPI.countAnalysisJobs())
}

func TestStudentChangesRemoveTheClone(t *testing.T) {
	testAPI := newTestAPI(t)
	anna := testAPI.register("anna", roleSupervisor, nil)
	student := anna.addStudent("Alice", "bachelor")
	studentPath := fmt.Sprintf("/students/%d", student.ID)
	cloneDir := testAPI.fakeClone(student.Repository.ID)

	sameURL := studentInput{FullName: "Alice Smith", Stream: "bachelor", CloneURL: student.Repository.CloneURL}
	require.Equal(t, http.StatusOK, anna.call(http.MethodPut, studentPath, sameURL).status)
	require.DirExists(t, cloneDir)

	newURL := studentInput{FullName: "Alice Smith", Stream: "bachelor", CloneURL: "git@gitlab.com:alice/thesis.git"}
	require.Equal(t, http.StatusOK, anna.call(http.MethodPut, studentPath, newURL).status)
	require.NoDirExists(t, cloneDir)

	testAPI.fakeClone(student.Repository.ID)
	require.Equal(t, http.StatusNoContent, anna.call(http.MethodDelete, studentPath, nil).status)
	require.NoDirExists(t, cloneDir)
}

func (testAPI *testAPI) fakeClone(repositoryID int64) string {
	testAPI.t.Helper()
	cloneDir := worker.ClonePath(testAPI.reposDir, repositoryID)
	require.NoError(testAPI.t, os.MkdirAll(cloneDir, 0o755))
	require.NoError(testAPI.t, os.WriteFile(filepath.Join(cloneDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644))
	return cloneDir
}

func (testAPI *testAPI) countAnalysisJobs() int {
	testAPI.t.Helper()
	var count int
	err := testAPI.pool.QueryRow(testAPI.t.Context(),
		"SELECT count(*) FROM river_job WHERE kind = $1", worker.AnalyzeArgs{}.Kind()).Scan(&count)
	require.NoError(testAPI.t, err)
	return count
}
