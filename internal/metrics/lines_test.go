package metrics_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"coursework/internal/metrics"
)

func linesByPath(report metrics.Report) map[string]metrics.LineCount {
	lines := map[string]metrics.LineCount{}
	for _, file := range report.Snapshot {
		lines[file.Path] = metrics.LineCount{Code: file.CodeLines, Comments: file.CommentLines}
	}
	return lines
}

func TestCommentLinesForAllFiveLanguages(t *testing.T) {
	report := analyzeFixture(t, "comments")

	require.Equal(t, map[string]metrics.LineCount{
		"src/index.php":     {Code: 5, Comments: 3},
		"src/sum.js":        {Code: 4, Comments: 2},
		"src/student.ts":    {Code: 7, Comments: 2},
		"src/total.py":      {Code: 3, Comments: 2},
		"src/app/Main.java": {Code: 6, Comments: 4},
	}, linesByPath(report))
	require.Equal(t, metrics.LineCounts{Code: 25, Comments: 13, Total: 38}, report.CodeStats.Lines)
}

func TestCountLines(t *testing.T) {
	testCases := map[string]struct {
		path     string
		content  string
		expected metrics.LineCount
	}{
		"tsx with block comment":           {"src/App.tsx", "/* App */\nexport const App = () => null;\n", metrics.LineCount{Code: 1, Comments: 1}},
		"mjs with line comment":            {"index.mjs", "// entry\nexport default 1;\n", metrics.LineCount{Code: 1, Comments: 1}},
		"uppercase extension":              {"Main.JAVA", "// entry\nclass Main {}\n", metrics.LineCount{Code: 1, Comments: 1}},
		"go has no comment split":          {"main.go", "// Package main\npackage main\n\n\nfunc main() {}\n", metrics.LineCount{Code: 3}},
		"markdown counts non-blank lines":  {"README.md", "# Title\n\n  \nText\n", metrics.LineCount{Code: 2}},
		"last line without newline counts": {"notes.txt", "one\ntwo", metrics.LineCount{Code: 2}},
		"empty file":                       {"empty.js", "", metrics.LineCount{}},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, testCase.expected, metrics.CountLines(testCase.path, []byte(testCase.content)))
		})
	}
}

func TestJunkIsExcludedFromCodeStats(t *testing.T) {
	report := analyzeFixture(t, "junk")

	require.Equal(t, metrics.LineCounts{Code: 10, Docs: 1, Total: 11}, report.CodeStats.Lines)
	require.Equal(t, 4, report.CodeStats.Junk.Files)
	require.Equal(t, 40+3+30, report.CodeStats.Junk.Lines)
	require.Positive(t, report.CodeStats.Junk.Bytes)
	require.Equal(t, []string{
		"assets/logo.png",
		"node_modules/left-pad/index.js",
		"node_modules/left-pad/package.json",
		"vendor/github.com/pkg/errors/errors.go",
	}, report.CodeStats.Junk.SamplePaths)
}

func TestCodeStatsWithCustomPatterns(t *testing.T) {
	files := analyzeFixture(t, "junk").Snapshot

	docsAsJunk := metrics.CodeStats(files, []string{"**/*.md"})
	require.Equal(t, metrics.LineCounts{Code: 10 + 40 + 30, Config: 3, Total: 83}, docsAsJunk.Lines)
	require.Equal(t, metrics.JunkVolume{Files: 1, Bytes: docsAsJunk.Junk.Bytes, Lines: 1, SamplePaths: []string{"README.md"}}, docsAsJunk.Junk)

	noPatterns := metrics.CodeStats(files, []string{})
	require.Equal(t, 0, noPatterns.Junk.Files)
	require.Equal(t, []string{}, noPatterns.Junk.SamplePaths)
}

func TestCodeStatsCategories(t *testing.T) {
	files := []metrics.SnapshotFile{
		{Path: "src/app.ts", Category: metrics.CategoryCode, CodeLines: 10, CommentLines: 2},
		{Path: "src/app.test.ts", Category: metrics.CategoryTests, CodeLines: 5, CommentLines: 1},
		{Path: "docs/plan.md", Category: metrics.CategoryDocs, CodeLines: 7},
		{Path: "package.json", Category: metrics.CategoryConfig, CodeLines: 4},
		{Path: "docs/note.pdf", Category: metrics.CategoryBinary, IsBinary: true, SizeBytes: 100},
	}

	stats := metrics.CodeStats(files, []string{"**/vendor/**"})
	require.Equal(t, metrics.LineCounts{Code: 10, Comments: 2, Docs: 7, Tests: 6, Config: 4, Total: 29}, stats.Lines)
	require.Equal(t, 0, stats.Junk.Files)
}

func TestJunkSamplePathsAreLimited(t *testing.T) {
	var files []metrics.SnapshotFile
	for i := range metrics.MaxJunkSamplePaths + 10 {
		files = append(files, metrics.SnapshotFile{Path: fmt.Sprintf("vendor/file%d.go", i), Category: metrics.CategoryCode, CodeLines: 1})
	}

	stats := metrics.CodeStats(files, metrics.DefaultJunkPatterns)
	require.Equal(t, metrics.MaxJunkSamplePaths+10, stats.Junk.Files)
	require.Equal(t, metrics.MaxJunkSamplePaths+10, stats.Junk.Lines)
	require.Len(t, stats.Junk.SamplePaths, metrics.MaxJunkSamplePaths)
}
