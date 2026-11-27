package metrics

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"slices"
	"time"
	_ "time/tzdata"

	"coursework/internal/git"
)

const TimeZone = "Europe/Kyiv"

var kyivLocation = loadLocation(TimeZone)

type CommitSummary struct {
	SHA          string    `json:"sha"`
	AuthorName   string    `json:"authorName"`
	AuthorEmail  string    `json:"authorEmail"`
	AuthoredAt   time.Time `json:"authoredAt"`
	Message      string    `json:"message"`
	IsMerge      bool      `json:"isMerge"`
	LinesAdded   int       `json:"linesAdded"`
	LinesDeleted int       `json:"linesDeleted"`
}

type DayActivity struct {
	Date         string `json:"date"`
	Commits      int    `json:"commits"`
	LinesChanged int    `json:"linesChanged"`
}

type SizeBucket struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

type CommitSizeStats struct {
	Buckets          []SizeBucket    `json:"buckets"`
	Mean             float64         `json:"mean"`
	Median           float64         `json:"median"`
	NonMergeCommits  int             `json:"nonMergeCommits"`
	LargeCommitRange CommitSizeRange `json:"largeCommitRange"`
	LargeCommits     []CommitSummary `json:"largeCommits"`
}

type RapidCommitSeries struct {
	StartedAt time.Time       `json:"startedAt"`
	EndedAt   time.Time       `json:"endedAt"`
	Commits   []CommitSummary `json:"commits"`
}

// Report is the output of the analyzer; later analysis steps add their metrics to it
type Report struct {
	Activity    []DayActivity       `json:"activity"`
	CommitSizes CommitSizeStats     `json:"commitSizes"`
	RapidSeries []RapidCommitSeries `json:"rapidSeries"`
	Snapshot    []SnapshotFile      `json:"snapshot"`
	Checklist   Checklist           `json:"checklist"`
	CodeStats   CodeStatistics      `json:"codeStats"`
}

func AnalyzeRepository(ctx context.Context, runner git.Runner, dir string) (Report, error) {
	commits, err := git.Log(ctx, runner, dir, "HEAD")
	if err != nil {
		return Report{}, err
	}

	snapshot, err := ReadSnapshot(ctx, runner, dir)
	if err != nil {
		return Report{}, err
	}

	return Report{
		Activity:    DailyActivity(commits),
		CommitSizes: CommitSizes(commits, DefaultCommitSizeSettings),
		RapidSeries: RapidSeries(commits, DefaultRapidSeriesSettings),
		Snapshot:    snapshot.Files,
		Checklist:   EvaluateChecklist(snapshot),
		CodeStats:   CodeStats(snapshot.Files, DefaultJunkPatterns),
	}, nil
}

func MarshalReport(report Report) ([]byte, error) {
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		return nil, err
	}

	return output.Bytes(), nil
}

func Summarize(commit git.Commit) CommitSummary {
	return CommitSummary{
		SHA:          commit.SHA,
		AuthorName:   commit.AuthorName,
		AuthorEmail:  commit.AuthorEmail,
		AuthoredAt:   commit.AuthoredAt,
		Message:      commit.Message,
		IsMerge:      commit.IsMerge,
		LinesAdded:   commit.LinesAdded,
		LinesDeleted: commit.LinesDeleted,
	}
}

// DailyActivity counts commits and changed lines per Kyiv day of the author date, merges included
func DailyActivity(commits []git.Commit) []DayActivity {
	activityByDate := map[string]*DayActivity{}
	for _, commit := range commits {
		date := kyivDate(commit.AuthoredAt)
		day, found := activityByDate[date]
		if !found {
			day = &DayActivity{Date: date}
			activityByDate[date] = day
		}
		day.Commits++
		day.LinesChanged += commitSize(commit)
	}

	days := make([]DayActivity, 0, len(activityByDate))
	for _, day := range activityByDate {
		days = append(days, *day)
	}

	slices.SortFunc(days, func(a, b DayActivity) int { return cmp.Compare(a.Date, b.Date) })
	return days
}

// CommitSizes builds size statistics over non-merge commits, junk files included; a commit outside every range is not in any bucket
func CommitSizes(commits []git.Commit, settings CommitSizeSettings) CommitSizeStats {
	largeCommitRange := settings.LargeCommitRange()
	sizes := CommitSizeStats{
		Buckets:          make([]SizeBucket, len(settings.Ranges)),
		LargeCommitRange: largeCommitRange,
		LargeCommits:     []CommitSummary{},
	}
	for i, sizeRange := range settings.Ranges {
		sizes.Buckets[i].Label = sizeRange.Label()
	}

	var commitLines []int
	for _, commit := range sortedByAuthorDate(commits) {
		if commit.IsMerge {
			continue
		}

		lines := commitSize(commit)
		commitLines = append(commitLines, lines)
		bucket := slices.IndexFunc(settings.Ranges, func(sizeRange CommitSizeRange) bool { return sizeRange.Contains(lines) })
		if bucket >= 0 {
			sizes.Buckets[bucket].Count++
		}
		if largeCommitRange.Contains(lines) {
			sizes.LargeCommits = append(sizes.LargeCommits, Summarize(commit))
		}
	}

	sizes.NonMergeCommits = len(commitLines)
	sizes.Mean = mean(commitLines)
	sizes.Median = median(commitLines)
	return sizes
}

// commitRun holds inclusive indexes of commits sorted by author date
type commitRun struct {
	first, last int
}

// RapidSeries joins the runs found by every enabled rule; runs that share a commit become one series
func RapidSeries(commits []git.Commit, settings RapidSeriesSettings) []RapidCommitSeries {
	sorted := sortedByAuthorDate(commits)
	var runs []commitRun
	if settings.Consecutive.Enabled {
		runs = append(runs, consecutiveRuns(sorted, settings.Consecutive)...)
	}
	if settings.Window.Enabled {
		runs = append(runs, windowRuns(sorted, settings.Window)...)
	}

	series := []RapidCommitSeries{}
	for _, run := range mergeOverlappingRuns(runs) {
		series = append(series, newRapidSeries(sorted[run.first:run.last+1]))
	}

	return series
}

// consecutiveRuns finds runs where every gap between neighbouring author dates is at most the rule's gap
func consecutiveRuns(sorted []git.Commit, rule ConsecutiveRule) []commitRun {
	maxGap := time.Duration(rule.MaxGapSeconds) * time.Second
	var runs []commitRun
	runStart := 0

	for i := 1; i <= len(sorted); i++ {
		isRunContinued := i < len(sorted) && sorted[i].AuthoredAt.Sub(sorted[i-1].AuthoredAt) <= maxGap
		if isRunContinued {
			continue
		}

		if i-runStart >= rule.MinCommits {
			runs = append(runs, commitRun{first: runStart, last: i - 1})
		}
		runStart = i
	}

	return runs
}

// windowRuns finds every run of enough commits whose first and last author dates fit into the rule's window
func windowRuns(sorted []git.Commit, rule WindowRule) []commitRun {
	window := time.Duration(rule.WindowSeconds) * time.Second
	var runs []commitRun
	runStart := 0

	for runEnd := range sorted {
		for sorted[runEnd].AuthoredAt.Sub(sorted[runStart].AuthoredAt) > window {
			runStart++
		}

		if runEnd-runStart+1 >= rule.MinCommits {
			runs = append(runs, commitRun{first: runStart, last: runEnd})
		}
	}

	return runs
}

func mergeOverlappingRuns(runs []commitRun) []commitRun {
	slices.SortFunc(runs, func(a, b commitRun) int { return cmp.Compare(a.first, b.first) })

	var merged []commitRun
	for _, run := range runs {
		isOverlappingPrevious := len(merged) > 0 && run.first <= merged[len(merged)-1].last
		if !isOverlappingPrevious {
			merged = append(merged, run)
			continue
		}

		previous := &merged[len(merged)-1]
		previous.last = max(previous.last, run.last)
	}

	return merged
}

func newRapidSeries(commits []git.Commit) RapidCommitSeries {
	series := RapidCommitSeries{
		StartedAt: commits[0].AuthoredAt,
		EndedAt:   commits[len(commits)-1].AuthoredAt,
	}

	for _, commit := range commits {
		series.Commits = append(series.Commits, Summarize(commit))
	}

	return series
}

// sortedByAuthorDate expects commits newest first as git log gives them; equal author dates keep the parent first
func sortedByAuthorDate(commits []git.Commit) []git.Commit {
	sorted := slices.Clone(commits)
	slices.Reverse(sorted)
	slices.SortStableFunc(sorted, func(a, b git.Commit) int { return a.AuthoredAt.Compare(b.AuthoredAt) })
	return sorted
}

func commitSize(commit git.Commit) int {
	return commit.LinesAdded + commit.LinesDeleted
}

func mean(values []int) float64 {
	if len(values) == 0 {
		return 0
	}

	total := 0
	for _, value := range values {
		total += value
	}

	return float64(total) / float64(len(values))
}

func median(values []int) float64 {
	if len(values) == 0 {
		return 0
	}

	sorted := slices.Sorted(slices.Values(values))
	middle := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return float64(sorted[middle])
	}

	return float64(sorted[middle-1]+sorted[middle]) / 2
}

// loadLocation cannot fail because the time zone database is embedded with time/tzdata
func loadLocation(name string) *time.Location {
	location, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return location
}
