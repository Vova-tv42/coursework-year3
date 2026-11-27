package metrics

import (
	"fmt"
	"slices"
)

// CommitSizeRange includes both limits; a nil To means the range has no upper limit
type CommitSizeRange struct {
	From int  `json:"from"`
	To   *int `json:"to"`
}

type CommitSizeSettings struct {
	Ranges []CommitSizeRange `json:"ranges"`
	// LargeCommitLines marks commits with more lines as large; nil uses the largest range
	LargeCommitLines *int `json:"largeCommitLines"`
}

type ConsecutiveRule struct {
	Enabled       bool `json:"enabled"`
	MinCommits    int  `json:"minCommits"`
	MaxGapSeconds int  `json:"maxGapSeconds"`
}

type WindowRule struct {
	Enabled       bool `json:"enabled"`
	MinCommits    int  `json:"minCommits"`
	WindowSeconds int  `json:"windowSeconds"`
}

type RapidSeriesSettings struct {
	Consecutive ConsecutiveRule `json:"consecutive"`
	Window      WindowRule      `json:"window"`
}

var DefaultCommitSizeSettings = CommitSizeSettings{
	Ranges: []CommitSizeRange{
		{From: 0, To: new(10)},
		{From: 11, To: new(100)},
		{From: 101, To: new(300)},
		{From: 301, To: new(1000)},
		{From: 1001},
	},
}

var DefaultRapidSeriesSettings = RapidSeriesSettings{
	Consecutive: ConsecutiveRule{Enabled: true, MinCommits: 3, MaxGapSeconds: 60},
	Window:      WindowRule{Enabled: false, MinCommits: 5, WindowSeconds: 300},
}

func (sizeRange CommitSizeRange) Contains(lines int) bool {
	isBelowUpperLimit := sizeRange.To == nil || lines <= *sizeRange.To
	return lines >= sizeRange.From && isBelowUpperLimit
}

func (sizeRange CommitSizeRange) Label() string {
	if sizeRange.To == nil {
		return fmt.Sprintf("%d+", sizeRange.From)
	}
	return fmt.Sprintf("%d-%d", sizeRange.From, *sizeRange.To)
}

// LargeCommitRange expects at least one range when no line limit is set
func (settings CommitSizeSettings) LargeCommitRange() CommitSizeRange {
	if settings.LargeCommitLines != nil {
		return CommitSizeRange{From: *settings.LargeCommitLines + 1}
	}

	return slices.MaxFunc(settings.Ranges, func(a, b CommitSizeRange) int { return a.From - b.From })
}
