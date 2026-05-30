package api

import "testing"

func TestClampSimilarDistance(t *testing.T) {
	cases := []struct {
		in, want int
	}{
		{-5, 0},
		{0, 0},
		{10, 10}, // schema default — unchanged
		{64, 64},
		{65, maxSimilarDistance},
		{1 << 20, maxSimilarDistance},
	}
	for _, c := range cases {
		if got := clampSimilarDistance(c.in); got != c.want {
			t.Errorf("clampSimilarDistance(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestClampSimilarLimit(t *testing.T) {
	cases := []struct {
		in, want int
	}{
		{-1, 0},
		{0, 0},
		{40, 40}, // schema default — unchanged
		{1000, 1000},
		{1001, maxSimilarLimit},
		{100000000, maxSimilarLimit},
	}
	for _, c := range cases {
		if got := clampSimilarLimit(c.in); got != c.want {
			t.Errorf("clampSimilarLimit(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}
