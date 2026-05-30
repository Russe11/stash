package loaders

import "testing"

// TestNormalizeFolderDepth pins the cache-key depth normalization: nil and any negative value must
// collapse to the single "unlimited" sentinel so every unlimited caller shares one key, while a
// non-negative depth is preserved verbatim.
func TestNormalizeFolderDepth(t *testing.T) {
	neg := -5
	zero := 0
	three := 3

	cases := []struct {
		name string
		in   *int
		want int
	}{
		{"nil is unlimited", nil, folderDepthUnlimited},
		{"negative is unlimited", &neg, folderDepthUnlimited},
		{"zero is self-only", &zero, 0},
		{"positive preserved", &three, 3},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := NormalizeFolderDepth(c.in); got != c.want {
				t.Errorf("NormalizeFolderDepth(%v) = %d, want %d", c.in, got, c.want)
			}
		})
	}
}

// TestDepthArgRoundTrip pins the inverse mapping used to call the store: the unlimited sentinel must
// become a nil *int (store's "no limit"), and a non-negative depth must round-trip to that exact
// value. A bug here would compute the WRONG count, not merely miss batching.
func TestDepthArgRoundTrip(t *testing.T) {
	if got := depthArg(folderDepthUnlimited); got != nil {
		t.Errorf("depthArg(unlimited) = %d, want nil", *got)
	}
	if got := depthArg(-1); got != nil {
		t.Errorf("depthArg(-1) = %d, want nil", *got)
	}
	for _, d := range []int{0, 1, 2, 10} {
		got := depthArg(d)
		if got == nil {
			t.Errorf("depthArg(%d) = nil, want &%d", d, d)
			continue
		}
		if *got != d {
			t.Errorf("depthArg(%d) = &%d, want &%d", d, *got, d)
		}
	}

	// NormalizeFolderDepth -> depthArg must reproduce the store's nil/value semantics.
	neg := -3
	four := 4
	if depthArg(NormalizeFolderDepth(nil)) != nil {
		t.Error("nil depth must round-trip to a nil store arg (unlimited)")
	}
	if depthArg(NormalizeFolderDepth(&neg)) != nil {
		t.Error("negative depth must round-trip to a nil store arg (unlimited)")
	}
	if got := depthArg(NormalizeFolderDepth(&four)); got == nil || *got != 4 {
		t.Errorf("depth 4 must round-trip to &4, got %v", got)
	}
}
