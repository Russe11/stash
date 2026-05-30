package api

import "testing"

func TestCapAllResults(t *testing.T) {
	t.Run("below the cap is unchanged", func(t *testing.T) {
		in := make([]int, maxAllResults-1)
		got := capAllResults("Test", in)
		if len(got) != maxAllResults-1 {
			t.Errorf("len = %d, want %d", len(got), maxAllResults-1)
		}
		// same backing slice, untruncated
		if &got[0] != &in[0] {
			t.Errorf("expected the input slice to be returned unchanged")
		}
	})

	t.Run("at the cap is unchanged", func(t *testing.T) {
		in := make([]int, maxAllResults)
		got := capAllResults("Test", in)
		if len(got) != maxAllResults {
			t.Errorf("len = %d, want %d", len(got), maxAllResults)
		}
		if &got[0] != &in[0] {
			t.Errorf("expected the input slice to be returned unchanged")
		}
	})

	t.Run("above the cap truncates to exactly maxAllResults", func(t *testing.T) {
		in := make([]int, maxAllResults+1)
		got := capAllResults("Test", in)
		if len(got) != maxAllResults {
			t.Errorf("len = %d, want %d", len(got), maxAllResults)
		}
	})

	t.Run("far above the cap truncates to exactly maxAllResults", func(t *testing.T) {
		in := make([]int, maxAllResults*3)
		got := capAllResults("Test", in)
		if len(got) != maxAllResults {
			t.Errorf("len = %d, want %d", len(got), maxAllResults)
		}
	})

	t.Run("nil and empty are unchanged", func(t *testing.T) {
		if got := capAllResults[int]("Test", nil); got != nil {
			t.Errorf("nil input: got len %d, want nil", len(got))
		}
		if got := capAllResults("Test", []int{}); len(got) != 0 {
			t.Errorf("empty input: got len %d, want 0", len(got))
		}
	})
}
