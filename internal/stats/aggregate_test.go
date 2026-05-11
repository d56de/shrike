package stats

import (
	"testing"
	"time"
)

func TestSummarize_StreakComputations(t *testing.T) {
	base := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	mkDay := func(off, scans int) Day { return Day{Date: base.AddDate(0, 0, off), Scans: scans} }

	cases := []struct {
		name           string
		days           []Day
		wantLongest    int
		wantCurrent    int
		wantActiveDays int
	}{
		{
			name:           "all gaps",
			days:           []Day{mkDay(0, 0), mkDay(1, 0), mkDay(2, 0)},
			wantLongest:    0,
			wantCurrent:    0,
			wantActiveDays: 0,
		},
		{
			name:           "single active day",
			days:           []Day{mkDay(0, 0), mkDay(1, 3), mkDay(2, 0)},
			wantLongest:    1,
			wantCurrent:    0,
			wantActiveDays: 1,
		},
		{
			name:           "current streak ends on last day",
			days:           []Day{mkDay(0, 0), mkDay(1, 1), mkDay(2, 1), mkDay(3, 1)},
			wantLongest:    3,
			wantCurrent:    3,
			wantActiveDays: 3,
		},
		{
			name:           "longest streak in the middle, no current",
			days:           []Day{mkDay(0, 1), mkDay(1, 1), mkDay(2, 1), mkDay(3, 1), mkDay(4, 0)},
			wantLongest:    4,
			wantCurrent:    0,
			wantActiveDays: 4,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := summarize(c.days, base, base.AddDate(0, 0, len(c.days)-1))
			if got.LongestStreak != c.wantLongest {
				t.Errorf("LongestStreak: got %d, want %d", got.LongestStreak, c.wantLongest)
			}
			if got.CurrentStreak != c.wantCurrent {
				t.Errorf("CurrentStreak: got %d, want %d", got.CurrentStreak, c.wantCurrent)
			}
			if got.ActiveDays != c.wantActiveDays {
				t.Errorf("ActiveDays: got %d, want %d", got.ActiveDays, c.wantActiveDays)
			}
		})
	}
}

func TestQuartileThresholds_EmptyAndSingle(t *testing.T) {
	// No activity → all zeros, level lookup will always return 0.
	if got := quartileThresholds(nil, "scans"); got != [3]int{0, 0, 0} {
		t.Errorf("empty input: got %v, want zeros", got)
	}
	if got := quartileThresholds([]Day{{Scans: 0}}, "scans"); got != [3]int{0, 0, 0} {
		t.Errorf("only-zero input: got %v, want zeros", got)
	}
	// Single non-zero value → all three thresholds equal it.
	if got := quartileThresholds([]Day{{Scans: 7}}, "scans"); got != [3]int{7, 7, 7} {
		t.Errorf("single value: got %v, want [7,7,7]", got)
	}
}

func TestWeekdayIndex(t *testing.T) {
	// Sun May 03 2026 is a Sunday → index 0.
	sun := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	if got := weekdayIndex(sun); got != 0 {
		t.Errorf("Sun should be 0, got %d", got)
	}
	sat := sun.AddDate(0, 0, 6)
	if got := weekdayIndex(sat); got != 6 {
		t.Errorf("Sat should be 6, got %d", got)
	}
}
