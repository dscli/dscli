package prompt

import (
	"testing"
	"time"
)

func TestFormatTime(t *testing.T) {
	now := time.Now()

	today := now.Format("15:04")
	if got := FormatTime(now); got != today {
		t.Errorf("FormatTime(today) = %s, want %s", got, today)
	}

	thisYear := time.Date(now.Year(), 6, 15, 10, 30, 0, 0, now.Location())
	wantThisYear := thisYear.Format("01-02 15:04")
	if got := FormatTime(thisYear); got != wantThisYear {
		t.Errorf("FormatTime(this year) = %s, want %s", got, wantThisYear)
	}

	otherYear := time.Date(2024, 12, 1, 8, 0, 0, 0, now.Location())
	wantOtherYear := otherYear.Format("2006-01-02 15:04")
	if got := FormatTime(otherYear); got != wantOtherYear {
		t.Errorf("FormatTime(other year) = %s, want %s", got, wantOtherYear)
	}
}
