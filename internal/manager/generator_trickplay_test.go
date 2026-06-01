package manager

import (
	"math"
	"testing"
)

func TestTrickplayInterval(t *testing.T) {
	// Sprite cadence = duration/81 (DefaultSpriteAmount).
	sprite := func(d float64) float64 { return d / float64(DefaultSpriteAmount) }

	cases := []struct {
		name         string
		duration     float64
		wantInterval float64
		wantMaxFrame int
	}{
		// Typical short scene: the 10s density floor governs (sprite cadence is finer).
		{"20min", 20 * 60, 10, 120},
		// 2h: the frame cap nudges spacing just past the floor (7200/600 = 12s).
		{"2h", 2 * 60 * 60, 12, 600},
		// 14h pathological file: cap dominates, frames stay bounded.
		{"14h", 50727, 50727.0 / 600.0, 600},
	}
	for _, c := range cases {
		got := trickplayInterval(c.duration, sprite(c.duration))
		if math.Abs(got-c.wantInterval) > 0.01 {
			t.Errorf("%s: interval = %.3f, want %.3f", c.name, got, c.wantInterval)
		}
		frames := int(math.Ceil(c.duration / got))
		if frames > c.wantMaxFrame {
			t.Errorf("%s: %d frames exceeds cap %d", c.name, frames, c.wantMaxFrame)
		}
	}
}

func TestTrickplayIntervalNeverExceedsCap(t *testing.T) {
	for _, d := range []float64{60, 600, 3600, 7200, 36000, 100000} {
		interval := trickplayInterval(d, d/float64(DefaultSpriteAmount))
		frames := int(math.Ceil(d / interval))
		if frames > DefaultTrickplayMaxFrames {
			t.Errorf("duration %.0fs produced %d frames, exceeds cap %d", d, frames, DefaultTrickplayMaxFrames)
		}
	}
}
