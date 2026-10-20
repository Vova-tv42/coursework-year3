package git_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"coursework/internal/fixture"
	"coursework/internal/git"
)

func buildFixture(t *testing.T, scenario string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, fixture.Build(scenario, dir))
	return dir
}

func logFixture(t *testing.T, scenario string) []git.Commit {
	t.Helper()
	commits, err := git.Log(t.Context(), git.ExecRunner{}, buildFixture(t, scenario), "HEAD")
	require.NoError(t, err)
	return commits
}

func findCommit(t *testing.T, commits []git.Commit, message string) git.Commit {
	t.Helper()
	for _, commit := range commits {
		if commit.Message == message {
			return commit
		}
	}
	require.FailNow(t, "commit not found", message)
	return git.Commit{}
}

func TestLogParsesCommitFields(t *testing.T) {
	commits := logFixture(t, "kyiv_boundary")
	require.Len(t, commits, 2)

	night, evening := commits[0], commits[1]
	require.Len(t, night.SHA, 40)
	require.Equal(t, []string{evening.SHA}, night.ParentSHAs)
	require.Empty(t, evening.ParentSHAs)
	require.Equal(t, fixture.Student.Name, night.AuthorName)
	require.Equal(t, fixture.Student.Email, night.AuthorEmail)
	require.True(t, night.AuthoredAt.Equal(time.Date(2026, 10, 5, 23, 30, 0, 0, time.UTC)))
	require.True(t, night.CommittedAt.Equal(night.AuthoredAt))
	require.Equal(t, "Night commit", night.Message)
	require.False(t, night.IsMerge)
	require.Equal(t, 3, night.LinesAdded)
	require.Equal(t, 0, night.LinesDeleted)
	require.Equal(t, []git.FileChange{{Path: "src/night.js", LinesAdded: 3}}, night.Files)
}

func TestLogKeepsNamesWithSpacesQuotesAndCyrillic(t *testing.T) {
	commits := logFixture(t, "names")
	require.Len(t, commits, 2)

	cyrillicCommit, quotedCommit := commits[0], commits[1]
	require.Equal(t, fixture.CyrillicAuthor.Name, cyrillicCommit.AuthorName)
	require.Equal(t, "Додати звіт\n\nОпис змін у тілі повідомлення.", cyrillicCommit.Message)
	require.ElementsMatch(t, []git.FileChange{
		{Path: `src/say "hello".js`, LinesAdded: 3},
		{Path: "документи/звіт.md", LinesAdded: 3},
	}, cyrillicCommit.Files)

	require.Equal(t, `Mary "Ann" O'Neil`, quotedCommit.AuthorName)
	require.Equal(t, `Add "quoted" file with spaces`, quotedCommit.Message)
	require.Equal(t, []git.FileChange{{Path: "src/file with spaces.js", LinesAdded: 2}}, quotedCommit.Files)
}

func TestLogCountsRenameUnderNewPath(t *testing.T) {
	rename := findCommit(t, logFixture(t, "merge_rename"), "Rename module")

	require.Equal(t, []git.FileChange{
		{Path: "src/new_name.js", OldPath: "src/old_name.js", LinesAdded: 1},
	}, rename.Files)
}

func TestLogMergeCommitHasNoFiles(t *testing.T) {
	merge := findCommit(t, logFixture(t, "merge_rename"), "Merge branch feature")

	require.True(t, merge.IsMerge)
	require.Len(t, merge.ParentSHAs, 2)
	require.Empty(t, merge.Files)
	require.Equal(t, 0, merge.LinesAdded+merge.LinesDeleted)
}

func TestLogMarksBinaryFiles(t *testing.T) {
	logo := findCommit(t, logFixture(t, "junk"), "Add logo")

	require.Equal(t, []git.FileChange{{Path: "assets/logo.png", IsBinary: true}}, logo.Files)
	require.Equal(t, 0, logo.LinesAdded)
}

func TestLogOfEmptyRepositoryHasNoCommits(t *testing.T) {
	require.Empty(t, logFixture(t, "empty"))
}

func TestParseLogRejectsBrokenHeader(t *testing.T) {
	_, err := git.ParseLog([]byte("\x1eabc\x1fonly two fields\x00"))
	require.Error(t, err)
}

func TestListTreeAndBlobReaderDetectBinaryFiles(t *testing.T) {
	dir := buildFixture(t, "junk")
	files, err := git.ListTree(t.Context(), git.ExecRunner{}, dir)
	require.NoError(t, err)
	require.Len(t, files, 6)

	blobs, err := git.OpenBlobReader(t.Context(), dir)
	require.NoError(t, err)
	defer func() { require.NoError(t, blobs.Close()) }()

	binaryPaths := []string{}
	for _, file := range files {
		content, err := blobs.Read(file.ObjectSHA)
		require.NoError(t, err)
		require.Len(t, content, int(file.SizeBytes), file.Path)
		if git.IsBinaryContent(content) {
			binaryPaths = append(binaryPaths, file.Path)
		}
	}
	require.Equal(t, []string{"assets/logo.png"}, binaryPaths)
}

func TestShowReturnsPatchOfCommit(t *testing.T) {
	dir := buildFixture(t, "merge_rename")
	commits, err := git.Log(t.Context(), git.ExecRunner{}, dir, "HEAD")
	require.NoError(t, err)
	rename := findCommit(t, commits, "Rename module")

	diff, truncated, err := git.Show(t.Context(), git.ExecRunner{}, dir, rename.SHA)
	require.NoError(t, err)
	require.False(t, truncated)
	require.Contains(t, diff, "rename from src/old_name.js")
	require.Contains(t, diff, "+module line 11")
}

type fixedOutputRunner struct {
	output []byte
}

func (r fixedOutputRunner) Run(context.Context, string, ...string) ([]byte, error) {
	return r.output, nil
}

func TestShowTruncatesLargeDiff(t *testing.T) {
	threeMegabyteDiff := strings.Repeat("+generated line\n", 3<<20/16)

	diff, truncated, err := git.Show(t.Context(), fixedOutputRunner{output: []byte(threeMegabyteDiff)}, "", "abc")
	require.NoError(t, err)
	require.True(t, truncated)
	require.LessOrEqual(t, len(diff), 2<<20)
	require.True(t, strings.HasSuffix(diff, "\n"), "diff must end on a full line")
}

func TestCloneBareThenFetchNewCommits(t *testing.T) {
	sourceDir := buildFixture(t, "merge_rename")
	cloneDir := filepath.Join(t.TempDir(), "repo.git")
	runner := git.ExecRunner{}

	require.NoError(t, git.CloneBare(t.Context(), runner, sourceDir, cloneDir))
	oldHead, err := git.HeadSHA(t.Context(), runner, cloneDir)
	require.NoError(t, err)
	branch, err := git.DefaultBranch(t.Context(), runner, cloneDir)
	require.NoError(t, err)
	require.Equal(t, "main", branch)

	addCommit := exec.Command("git", "-c", "user.name=Student", "-c", "user.email=student@example.com",
		"commit", "--allow-empty", "--quiet", "--message", "Next step")
	addCommit.Dir = sourceDir
	addCommit.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	require.NoError(t, addCommit.Run())

	require.NoError(t, git.Fetch(t.Context(), runner, cloneDir))
	newHead, err := git.HeadSHA(t.Context(), runner, cloneDir)
	require.NoError(t, err)
	require.NotEqual(t, oldHead, newHead)

	isFastForward, err := git.IsAncestor(t.Context(), runner, cloneDir, oldHead, newHead)
	require.NoError(t, err)
	require.True(t, isFastForward)
	isRewound, err := git.IsAncestor(t.Context(), runner, cloneDir, newHead, oldHead)
	require.NoError(t, err)
	require.False(t, isRewound)

	sizeBytes, err := git.DirSize(cloneDir)
	require.NoError(t, err)
	require.Positive(t, sizeBytes)
}

func TestExecRunnerSendsTokenAsAuthorizationHeader(t *testing.T) {
	output, err := git.ExecRunner{Token: "secret"}.Run(t.Context(), "", "config", "--get", "http.extraHeader")
	require.NoError(t, err)

	// base64 of "x-access-token:secret"
	require.Equal(t, "Authorization: Basic eC1hY2Nlc3MtdG9rZW46c2VjcmV0\n", string(output))
}
