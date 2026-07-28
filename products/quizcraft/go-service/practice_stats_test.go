package quizcraft

import (
	"testing"
	"time"
)

func TestConsecutivePracticeDaysUsesAsiaShanghaiCalendar(t *testing.T) {
	now := time.Date(2026, time.July, 28, 0, 30, 0, 0, time.UTC) // 08:30 in Shanghai.

	for _, test := range []struct {
		name string
		days []string
		want int
	}{
		{name: "today and yesterday", days: []string{"2026-07-28", "2026-07-27"}, want: 2},
		{name: "yesterday starts a streak", days: []string{"2026-07-27"}, want: 1},
		{name: "a stale day is not a current streak", days: []string{"2026-07-26", "2026-07-25"}, want: 0},
		{name: "no activity", days: nil, want: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := consecutivePracticeDays(test.days, now)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("consecutivePracticeDays(%v) = %d, want %d", test.days, got, test.want)
			}
		})
	}
}

func TestConsecutivePracticeDaysRejectsInvalidStoredDates(t *testing.T) {
	if _, err := consecutivePracticeDays([]string{"not-a-date"}, time.Now()); err == nil {
		t.Fatal("invalid stored practice date was accepted")
	}
}
