package metrics

import (
	"slices"
	"time"

	"coursework/internal/git"
)

const PeriodKindPractice = "practice"

// CalendarPeriod keeps dates as "2006-01-02" strings, so they compare in date order as plain strings
type CalendarPeriod struct {
	ID        int64   `json:"id"`
	Name      string  `json:"name"`
	Kind      string  `json:"kind"`
	Stream    string  `json:"stream"`
	StartDate *string `json:"startDate"`
	EndDate   *string `json:"endDate"`
}

type PeriodActivitySummary struct {
	CalendarPeriod
	Commits          int  `json:"commits"`
	LinesChanged     int  `json:"linesChanged"`
	NoActivitySignal bool `json:"noActivitySignal"`
}

// PeriodActivity counts commits and changed lines inside the period by the Kyiv day of the author date
func PeriodActivity(commits []git.Commit, period CalendarPeriod, today time.Time) PeriodActivitySummary {
	summary := PeriodActivitySummary{CalendarPeriod: period}
	if !period.hasDates() {
		return summary
	}

	for _, commit := range commits {
		if !isDateBetween(kyivDate(commit.AuthoredAt), *period.StartDate, *period.EndDate) {
			continue
		}

		summary.Commits++
		summary.LinesChanged += commitSize(commit)
	}

	summary.NoActivitySignal = HasNoPracticeActivity(commits, period, today)
	return summary
}

// HasNoPracticeActivity is true for a started practice period without commits from its start up to today
func HasNoPracticeActivity(commits []git.Commit, period CalendarPeriod, today time.Time) bool {
	isPracticeWithDates := period.Kind == PeriodKindPractice && period.hasDates()
	if !isPracticeWithDates {
		return false
	}

	todayDate := kyivDate(today)
	hasPeriodStarted := *period.StartDate <= todayDate
	if !hasPeriodStarted {
		return false
	}

	lastCheckedDate := min(*period.EndDate, todayDate)
	hasCommitInPeriod := slices.ContainsFunc(commits, func(commit git.Commit) bool {
		return isDateBetween(kyivDate(commit.AuthoredAt), *period.StartDate, lastCheckedDate)
	})

	return !hasCommitInPeriod
}

func (period CalendarPeriod) hasDates() bool {
	return period.StartDate != nil && period.EndDate != nil
}

func isDateBetween(date, firstDate, lastDate string) bool {
	return firstDate <= date && date <= lastDate
}

func kyivDate(moment time.Time) string {
	return moment.In(kyivLocation).Format(time.DateOnly)
}
