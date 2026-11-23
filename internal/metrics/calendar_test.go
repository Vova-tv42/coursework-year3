package metrics_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"coursework/internal/git"
	"coursework/internal/metrics"
)

func newPeriod(kind, startDate, endDate string) metrics.CalendarPeriod {
	return metrics.CalendarPeriod{Name: "Practice", Kind: kind, Stream: "bachelor", StartDate: &startDate, EndDate: &endDate}
}

func commitAt(moment time.Time, linesAdded int) git.Commit {
	return git.Commit{SHA: moment.String(), AuthoredAt: moment, LinesAdded: linesAdded}
}

func TestPeriodActivityUsesKyivDates(t *testing.T) {
	commits := []git.Commit{
		commitAt(time.Date(2026, 10, 5, 23, 30, 0, 0, time.UTC), 7),
		commitAt(time.Date(2026, 10, 5, 20, 30, 0, 0, time.UTC), 3),
		commitAt(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC), 100),
	}
	today := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

	// 23:30 UTC on October 5 is already October 6 in Kyiv
	endsOnOctober5 := metrics.PeriodActivity(commits, newPeriod("work_start", "2026-10-01", "2026-10-05"), today)
	require.Equal(t, 1, endsOnOctober5.Commits)
	require.Equal(t, 3, endsOnOctober5.LinesChanged)

	startsOnOctober6 := metrics.PeriodActivity(commits, newPeriod("work_start", "2026-10-06", "2026-10-10"), today)
	require.Equal(t, 1, startsOnOctober6.Commits)
	require.Equal(t, 7, startsOnOctober6.LinesChanged)
	require.False(t, startsOnOctober6.NoActivitySignal)
}

func TestPeriodWithoutDatesHasNoActivity(t *testing.T) {
	commits := []git.Commit{commitAt(time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC), 5)}
	period := metrics.CalendarPeriod{Name: "Master practice", Kind: metrics.PeriodKindPractice, Stream: "master"}

	summary := metrics.PeriodActivity(commits, period, time.Date(2027, 6, 1, 12, 0, 0, 0, time.UTC))

	require.Equal(t, metrics.PeriodActivitySummary{CalendarPeriod: period}, summary)
}

func TestHasNoPracticeActivity(t *testing.T) {
	commits := []git.Commit{
		commitAt(time.Date(2027, 3, 31, 22, 30, 0, 0, time.UTC), 10),
		commitAt(time.Date(2027, 6, 15, 12, 0, 0, 0, time.UTC), 10),
	}
	midPractice := time.Date(2027, 4, 20, 12, 0, 0, 0, time.UTC)

	testCases := []struct {
		name     string
		commits  []git.Commit
		period   metrics.CalendarPeriod
		today    time.Time
		expected bool
	}{
		{
			name:     "practice not started",
			period:   newPeriod(metrics.PeriodKindPractice, "2027-04-01", "2027-05-31"),
			today:    time.Date(2027, 3, 31, 12, 0, 0, 0, time.UTC),
			expected: false,
		},
		{
			name:     "started without commits",
			period:   newPeriod(metrics.PeriodKindPractice, "2027-04-01", "2027-05-31"),
			today:    midPractice,
			expected: true,
		},
		{
			// 22:30 UTC on March 31 is April 1 in Kyiv
			name:     "commit on the Kyiv first day",
			commits:  commits,
			period:   newPeriod(metrics.PeriodKindPractice, "2027-04-01", "2027-05-31"),
			today:    midPractice,
			expected: false,
		},
		{
			name:     "commits only outside the finished period",
			commits:  commits,
			period:   newPeriod(metrics.PeriodKindPractice, "2027-04-02", "2027-05-31"),
			today:    time.Date(2027, 7, 1, 12, 0, 0, 0, time.UTC),
			expected: true,
		},
		{
			name:     "started today",
			period:   newPeriod(metrics.PeriodKindPractice, "2027-04-20", "2027-05-31"),
			today:    midPractice,
			expected: true,
		},
		{
			name:     "period without dates",
			period:   metrics.CalendarPeriod{Name: "Master practice", Kind: metrics.PeriodKindPractice, Stream: "master"},
			today:    midPractice,
			expected: false,
		},
		{
			name:     "not a practice period",
			period:   newPeriod("work_start", "2027-04-01", "2027-04-14"),
			today:    midPractice,
			expected: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			require.Equal(t, testCase.expected, metrics.HasNoPracticeActivity(testCase.commits, testCase.period, testCase.today))
		})
	}
}
