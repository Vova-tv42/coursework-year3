package api

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"coursework/internal/git"
	"coursework/internal/metrics"
	"coursework/internal/worker"
)

func findCommitByMessage(t *testing.T, commits []git.Commit, message string) git.Commit {
	t.Helper()
	for _, commit := range commits {
		if strings.TrimSpace(commit.Message) == message {
			return commit
		}
	}

	require.FailNow(t, "commit not found", message)
	return git.Commit{}
}

func TestTopFilesAreSortedByModifications(t *testing.T) {
	testAPI := newTestAPI(t)
	anna := testAPI.register("anna", roleSupervisor, nil)
	student, sourceCommits := testAPI.addAnalyzedStudent(anna, "Alice", "rapid")

	response := anna.call(http.MethodGet, fmt.Sprintf("/repos/%d/files", student.Repository.ID), nil)
	require.Equal(t, http.StatusOK, response.status, string(response.body))
	files := decode[[]fileModifications](t, response)

	type fileRow struct {
		path          string
		modifications int
		linesAdded    int
		linesDeleted  int
	}
	rows := make([]fileRow, 0, len(files))
	for _, file := range files {
		rows = append(rows, fileRow{file.Path, file.Modifications, file.LinesAdded, file.LinesDeleted})
	}

	// form.js and report.js have the same count, so the path decides their order
	require.Equal(t, []fileRow{
		{"src/form.js", 4, 4, 0},
		{"src/report.js", 4, 4, 0},
		{"src/api.js", 2, 6, 0},
	}, rows)

	lastReportCommit := findCommitByMessage(t, sourceCommits, "Extend report 4")
	require.True(t, lastReportCommit.AuthoredAt.Equal(files[1].LastModifiedAt))
}

func TestFileCommitsAreNewestFirst(t *testing.T) {
	testAPI := newTestAPI(t)
	anna := testAPI.register("anna", roleSupervisor, nil)
	student, _ := testAPI.addAnalyzedStudent(anna, "Alice", "rapid")
	fileCommitsPath := fmt.Sprintf("/repos/%d/file-commits?path=", student.Repository.ID)

	response := anna.call(http.MethodGet, fileCommitsPath+url.QueryEscape("src/report.js"), nil)
	require.Equal(t, http.StatusOK, response.status, string(response.body))
	var messages []string
	for _, commit := range decode[[]metrics.CommitSummary](t, response) {
		messages = append(messages, strings.TrimSpace(commit.Message))
	}

	require.Equal(t, []string{"Extend report 4", "Extend report 3", "Extend report 2", "Extend report 1"}, messages)

	response = anna.call(http.MethodGet, fileCommitsPath+"missing.js", nil)
	require.Equal(t, http.StatusOK, response.status, string(response.body))
	require.JSONEq(t, `[]`, string(response.body))

	requireError(t, anna.call(http.MethodGet, fileCommitsPath, nil), http.StatusBadRequest, "invalid_input")
}

func TestCommitDetailsListTouchedFiles(t *testing.T) {
	testAPI := newTestAPI(t)
	anna := testAPI.register("anna", roleSupervisor, nil)
	student, sourceCommits := testAPI.addAnalyzedStudent(anna, "Alice", "merge_rename")
	commitsPath := fmt.Sprintf("/repos/%d/commits/", student.Repository.ID)

	renameCommit := findCommitByMessage(t, sourceCommits, "Rename module")
	response := anna.call(http.MethodGet, commitsPath+renameCommit.SHA, nil)
	require.Equal(t, http.StatusOK, response.status, string(response.body))
	details := decode[commitDetails](t, response)

	oldPath := "src/old_name.js"
	require.Equal(t, renameCommit.SHA, details.SHA)
	require.False(t, details.IsMerge)
	require.Equal(t, []commitFile{
		{Path: "src/new_name.js", OldPath: &oldPath, LinesAdded: 1, LinesDeleted: 0},
	}, details.Files)

	mergeCommit := findCommitByMessage(t, sourceCommits, "Merge branch feature")
	response = anna.call(http.MethodGet, commitsPath+mergeCommit.SHA, nil)
	require.Equal(t, http.StatusOK, response.status, string(response.body))
	require.True(t, decode[commitDetails](t, response).IsMerge)
	require.Contains(t, string(response.body), `"files":[]`)

	requireError(t, anna.call(http.MethodGet, commitsPath+strings.Repeat("0", 40), nil), http.StatusNotFound, "not_found")
}

func TestCommitDiffComesFromTheClone(t *testing.T) {
	testAPI := newTestAPI(t)
	anna := testAPI.register("anna", roleSupervisor, nil)
	student, sourceCommits := testAPI.addAnalyzedStudent(anna, "Alice", "merge_rename")

	renameCommit := findCommitByMessage(t, sourceCommits, "Rename module")
	diffPath := fmt.Sprintf("/repos/%d/commits/%s/diff", student.Repository.ID, renameCommit.SHA)
	response := anna.call(http.MethodGet, diffPath, nil)
	require.Equal(t, http.StatusOK, response.status, string(response.body))
	diff := decode[commitDiff](t, response)

	require.False(t, diff.Truncated)
	require.Contains(t, diff.Diff, "rename from src/old_name.js")
	require.Contains(t, diff.Diff, "rename to src/new_name.js")
	require.Contains(t, diff.Diff, "+module line 11")

	require.NoError(t, os.RemoveAll(worker.ClonePath(testAPI.reposDir, student.Repository.ID)))
	requireError(t, anna.call(http.MethodGet, diffPath, nil), http.StatusNotFound, "not_found")
}

func TestCommitOfAnotherRepositoryIsNotFound(t *testing.T) {
	testAPI := newTestAPI(t)
	anna := testAPI.register("anna", roleSupervisor, nil)
	alice, aliceCommits := testAPI.addAnalyzedStudent(anna, "Alice", "merge_rename")
	bob, _ := testAPI.addAnalyzedStudent(anna, "Bob", "rapid")

	aliceCommit := findCommitByMessage(t, aliceCommits, "Rename module")
	bobCommitPath := fmt.Sprintf("/repos/%d/commits/%s", bob.Repository.ID, aliceCommit.SHA)
	requireError(t, anna.call(http.MethodGet, bobCommitPath, nil), http.StatusNotFound, "not_found")
	requireError(t, anna.call(http.MethodGet, bobCommitPath+"/diff", nil), http.StatusNotFound, "not_found")

	aliceCommitPath := fmt.Sprintf("/repos/%d/commits/%s", alice.Repository.ID, aliceCommit.SHA)
	require.Equal(t, http.StatusOK, anna.call(http.MethodGet, aliceCommitPath, nil).status)
}

func TestFilesAndCommitsOfForeignRepositoryAreNotFound(t *testing.T) {
	testAPI := newTestAPI(t)
	anna := testAPI.register("anna", roleSupervisor, nil)
	boris := testAPI.register("boris", roleSupervisor, nil)
	masterCoordinator := testAPI.register("masters", roleCoordinator, streamOf("master"))
	student, sourceCommits := testAPI.addAnalyzedStudent(anna, "Alice", "merge_rename")

	repositoryPath := fmt.Sprintf("/repos/%d", student.Repository.ID)
	commitPath := repositoryPath + "/commits/" + sourceCommits[0].SHA
	foreignPaths := []string{
		repositoryPath + "/files",
		repositoryPath + "/file-commits?path=src/other.js",
		commitPath,
		commitPath + "/diff",
	}

	for _, outsider := range []*apiClient{boris, masterCoordinator} {
		for _, path := range foreignPaths {
			requireError(t, outsider.call(http.MethodGet, path, nil), http.StatusNotFound, "not_found")
		}
	}
}
