package metrics_test

import (
	"flag"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"coursework/internal/fixture"
	"coursework/internal/git"
	"coursework/internal/metrics"
)

var update = flag.Bool("update", false, "rewrite golden files in testdata")

func analyzeFixture(t *testing.T, scenario string) metrics.Report {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, fixture.Build(scenario, dir))
	report, err := metrics.AnalyzeRepository(t.Context(), git.ExecRunner{}, dir)
	require.NoError(t, err)
	return report
}

func fixtureCommits(t *testing.T, scenario string) []git.Commit {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, fixture.Build(scenario, dir))
	commits, err := git.Log(t.Context(), git.ExecRunner{}, dir, "HEAD")
	require.NoError(t, err)
	return commits
}

func seriesMessages(series []metrics.RapidCommitSeries) [][]string {
	var messages [][]string
	for _, oneSeries := range series {
		var seriesMessages []string
		for _, commit := range oneSeries.Commits {
			seriesMessages = append(seriesMessages, commit.Message)
		}
		messages = append(messages, seriesMessages)
	}
	return messages
}

func marshalReport(t *testing.T, report metrics.Report) []byte {
	t.Helper()
	output, err := metrics.MarshalReport(report)
	require.NoError(t, err)
	return output
}

func TestReportMatchesGoldenFiles(t *testing.T) {
	for _, scenario := range slices.Sorted(maps.Keys(fixture.Scenarios)) {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			output := marshalReport(t, analyzeFixture(t, scenario))
			goldenPath := filepath.Join("testdata", scenario+".golden.json")

			if *update {
				require.NoError(t, os.WriteFile(goldenPath, output, 0o644))
				return
			}

			golden, err := os.ReadFile(goldenPath)
			require.NoError(t, err)
			require.Equal(t, string(golden), string(output))
		})
	}
}

func TestCommitSizeBuckets(t *testing.T) {
	sizes := analyzeFixture(t, "sizes").CommitSizes

	require.Equal(t, []metrics.SizeBucket{
		{Label: "0-10", Count: 20},
		{Label: "11-100", Count: 0},
		{Label: "101-300", Count: 20},
		{Label: "301-1000", Count: 0},
		{Label: "1001+", Count: 1},
	}, sizes.Buckets)
	require.Equal(t, 41, sizes.NonMergeCommits)
	require.InDelta(t, 5560.0/41, sizes.Mean, 1e-9)
	require.InDelta(t, 200.0, sizes.Median, 1e-9)
	require.Len(t, sizes.LargeCommits, 1)
	require.Equal(t, "Add generated data", sizes.LargeCommits[0].Message)
}

func TestCommitSizeRangesWithGapUseLargestRangeForLargeCommits(t *testing.T) {
	settings := metrics.CommitSizeSettings{
		Ranges: []metrics.CommitSizeRange{{From: 0, To: new(10)}, {From: 101, To: new(300)}},
	}

	sizes := metrics.CommitSizes(fixtureCommits(t, "sizes"), settings)

	require.Equal(t, []metrics.SizeBucket{{Label: "0-10", Count: 20}, {Label: "101-300", Count: 20}}, sizes.Buckets)
	require.Equal(t, 41, sizes.NonMergeCommits)
	require.Equal(t, metrics.CommitSizeRange{From: 101, To: new(300)}, sizes.LargeCommitRange)
	require.Len(t, sizes.LargeCommits, 20)
}

func TestLargeCommitLinesOverrideTheLargestRange(t *testing.T) {
	settings := metrics.DefaultCommitSizeSettings
	settings.LargeCommitLines = new(300)

	sizes := metrics.CommitSizes(fixtureCommits(t, "sizes"), settings)

	require.Equal(t, metrics.CommitSizeRange{From: 301}, sizes.LargeCommitRange)
	require.Len(t, sizes.LargeCommits, 1)
	require.Equal(t, "Add generated data", sizes.LargeCommits[0].Message)
}

func TestMergesAreExcludedFromSizesButCountedInActivity(t *testing.T) {
	report := analyzeFixture(t, "merge_rename")

	require.Equal(t, 3, report.CommitSizes.NonMergeCommits)
	totalCommits := 0
	for _, day := range report.Activity {
		totalCommits += day.Commits
	}

	require.Equal(t, 4, totalCommits)
}

func TestRapidSeriesFindsOnlyTheQuickRun(t *testing.T) {
	series := analyzeFixture(t, "rapid").RapidSeries

	require.Len(t, series, 1)
	var messages []string
	for _, commit := range series[0].Commits {
		messages = append(messages, commit.Message)
	}

	require.Equal(t, []string{"Tweak form 1", "Tweak form 2", "Tweak form 3", "Tweak form 4"}, messages)
	require.Equal(t, series[0].Commits[0].AuthoredAt, series[0].StartedAt)
	require.Equal(t, series[0].Commits[3].AuthoredAt, series[0].EndedAt)
}

func TestWindowRuleJoinsOverlappingWindows(t *testing.T) {
	settings := metrics.RapidSeriesSettings{
		Window: metrics.WindowRule{Enabled: true, MinCommits: 2, WindowSeconds: 30},
	}

	series := metrics.RapidSeries(fixtureCommits(t, "rapid"), settings)

	require.Equal(t, [][]string{
		{"Tweak form 1", "Tweak form 2", "Tweak form 3"},
		{"Add api client", "Fix api client"},
	}, seriesMessages(series))
}

func TestBothRapidSeriesRulesAddTheirSeries(t *testing.T) {
	settings := metrics.DefaultRapidSeriesSettings
	settings.Window = metrics.WindowRule{Enabled: true, MinCommits: 4, WindowSeconds: 60 * 60}

	series := metrics.RapidSeries(fixtureCommits(t, "rapid"), settings)

	require.Equal(t, [][]string{
		{"Tweak form 1", "Tweak form 2", "Tweak form 3", "Tweak form 4"},
		{"Extend report 1", "Extend report 2", "Extend report 3", "Extend report 4"},
	}, seriesMessages(series))
}

func TestRuleSeriesSharingCommitsAreMerged(t *testing.T) {
	settings := metrics.RapidSeriesSettings{
		Consecutive: metrics.ConsecutiveRule{Enabled: true, MinCommits: 2, MaxGapSeconds: 20},
		Window:      metrics.WindowRule{Enabled: true, MinCommits: 2, WindowSeconds: 60},
	}

	series := metrics.RapidSeries(fixtureCommits(t, "rapid"), settings)

	require.Equal(t, [][]string{
		{"Tweak form 1", "Tweak form 2", "Tweak form 3", "Tweak form 4"},
		{"Add api client", "Fix api client"},
	}, seriesMessages(series))
}

func TestActivityUsesKyivDays(t *testing.T) {
	activity := analyzeFixture(t, "kyiv_boundary").Activity

	require.Equal(t, []metrics.DayActivity{
		{Date: "2026-10-05", Commits: 1, LinesChanged: 2},
		{Date: "2026-10-06", Commits: 1, LinesChanged: 3},
	}, activity)
}

func TestEmptyRepositoryGivesEmptyMetrics(t *testing.T) {
	report := analyzeFixture(t, "empty")

	require.Empty(t, report.Activity)
	require.Empty(t, report.RapidSeries)
	require.Zero(t, report.CommitSizes.NonMergeCommits)
	require.Zero(t, report.CommitSizes.Mean)
	require.Zero(t, report.CommitSizes.Median)
}

func TestTwoRunsGiveIdenticalOutput(t *testing.T) {
	first := marshalReport(t, analyzeFixture(t, "rapid"))
	second := marshalReport(t, analyzeFixture(t, "rapid"))

	require.Equal(t, first, second)
}
