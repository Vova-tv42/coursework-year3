package metrics

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"math"
	"slices"
	"time"
	_ "time/tzdata"

	"coursework/internal/git"
)

const (
	TimeZone              = "Europe/Kyiv"
	LargeCommitLines      = 1000
	RapidSeriesMinCommits = 3
	RapidSeriesMaxGap     = 60 * time.Second
)

var kyivLocation = loadLocation(TimeZone)

type sizeBucketLimit struct {
	label    string
	maxLines int
}

var sizeBuckets = []sizeBucketLimit{
	{label: "0", maxLines: 0},
	{label: "1-10", maxLines: 10},
	{label: "11-50", maxLines: 50},
	{label: "51-200", maxLines: 200},
	{label: "201-1000", maxLines: 1000},
	{label: ">1000", maxLines: math.MaxInt},
}

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
	Buckets         []SizeBucket    `json:"buckets"`
	Mean            float64         `json:"mean"`
	Median          float64         `json:"median"`
	NonMergeCommits int             `json:"nonMergeCommits"`
	LargeCommits    []CommitSummary `json:"largeCommits"`
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
}

func AnalyzeRepository(ctx context.Context, runner git.Runner, dir string) (Report, error) {
	commits, err := git.Log(ctx, runner, dir, "HEAD")
	if err != nil {
		return Report{}, err
	}
	return Report{
		Activity:    DailyActivity(commits),
		CommitSizes: CommitSizes(commits),
		RapidSeries: RapidSeries(commits),
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
		date := commit.AuthoredAt.In(kyivLocation).Format(time.DateOnly)
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

// CommitSizes builds size statistics over non-merge commits, junk files included
func CommitSizes(commits []git.Commit) CommitSizeStats {
	sizes := CommitSizeStats{
		Buckets:      make([]SizeBucket, len(sizeBuckets)),
		LargeCommits: []CommitSummary{},
	}
	for i, bucket := range sizeBuckets {
		sizes.Buckets[i].Label = bucket.label
	}

	var commitLines []int
	for _, commit := range sortedByAuthorDate(commits) {
		if commit.IsMerge {
			continue
		}

		lines := commitSize(commit)
		commitLines = append(commitLines, lines)
		sizes.Buckets[bucketIndex(lines)].Count++
		if lines > LargeCommitLines {
			sizes.LargeCommits = append(sizes.LargeCommits, Summarize(commit))
		}
	}

	sizes.NonMergeCommits = len(commitLines)
	sizes.Mean = mean(commitLines)
	sizes.Median = median(commitLines)
	return sizes
}

// RapidSeries returns runs of at least three commits where every gap between author dates is at most a minute
func RapidSeries(commits []git.Commit) []RapidCommitSeries {
	sorted := sortedByAuthorDate(commits)
	series := []RapidCommitSeries{}
	runStart := 0

	for i := 1; i <= len(sorted); i++ {
		isRunContinued := i < len(sorted) && sorted[i].AuthoredAt.Sub(sorted[i-1].AuthoredAt) <= RapidSeriesMaxGap
		if isRunContinued {
			continue
		}

		if i-runStart >= RapidSeriesMinCommits {
			series = append(series, newRapidSeries(sorted[runStart:i]))
		}
		runStart = i
	}

	return series
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

func bucketIndex(lines int) int {
	return slices.IndexFunc(sizeBuckets, func(bucket sizeBucketLimit) bool { return lines <= bucket.maxLines })
}

func mean(values []int) float64 {
	if len(values) == 0 {
		return 0
	}

	total := 0
	for _, value := range values {
		total += value


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
