package backup

import (
	"testing"
	"time"
)

func TestNextSlot(t *testing.T) {
	paris, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Fatal(err)
	}
	at := func(s string) time.Time {
		v, err := time.ParseInLocation("2006-01-02 15:04", s, paris)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	for _, c := range []struct{ from, freq, want string }{
		{"2026-11-07 02:59", Daily, "2026-11-07 03:00"},
		{"2026-11-07 03:00", Daily, "2026-11-08 03:00"},
		{"2026-11-07 12:00", Daily, "2026-11-08 03:00"},
		// Saturday: the next Sunday; a Sunday after 03:00: the one after.
		{"2026-11-07 12:00", Weekly, "2026-11-08 03:00"},
		{"2026-11-08 03:00", Weekly, "2026-11-15 03:00"},
		// Across the change to winter time.
		{"2026-10-24 12:00", Daily, "2026-10-25 03:00"},
	} {
		if got := NextSlot(at(c.from), c.freq); !got.Equal(at(c.want)) {
			t.Errorf("NextSlot(%s, %s) = %s, want %s", c.from, c.freq, got, c.want)
		}
	}
}

func TestExpired(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	days := func(n ...int) []time.Time {
		out := make([]time.Time, len(n))
		for i, d := range n {
			out[i] = now.Add(-time.Duration(d)*24*time.Hour - time.Hour)
		}
		return out
	}
	all := days(0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 13, 14, 20, 21, 27, 28, 34, 35, 40)
	count := func(retention string) int { return len(all) - len(Expired(all, retention, now)) }
	if n := count(KeepLast3); n != 3 {
		t.Errorf("last3 keeps %d", n)
	}
	// 0 to 6, then one per week for four weeks.
	if n := count(Keep7d4w); n != 7+4 {
		t.Errorf("7d4w keeps %d", n)
	}
	if n := count(Keep30d); n != 17 {
		t.Errorf("30d keeps %d", n)
	}
	// The newest stays, however old.
	if got := Expired(days(400), Keep30d, now); len(got) != 0 {
		t.Errorf("newest expired: %v", got)
	}
}
