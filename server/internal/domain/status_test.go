package domain

import "testing"

func TestEffectiveStatus(t *testing.T) {
	cases := []struct {
		observed  Status
		reachable bool
		want      Status
	}{
		{StatusOnline, true, StatusOnline},
		{StatusDown, true, StatusDown},
		{StatusOnline, false, StatusUnknown},
		{StatusDown, false, StatusUnknown},
		{StatusConfigRequired, false, StatusUnknown},
	}
	for _, c := range cases {
		if got := EffectiveStatus(c.observed, c.reachable); got != c.want {
			t.Errorf("EffectiveStatus(%q, %v) = %q, want %q", c.observed, c.reachable, got, c.want)
		}
	}
}
