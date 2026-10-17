package reqbody

import (
	"math"
	"testing"
)

func TestFitStretchesToOneFile(t *testing.T) {
	for _, tc := range []struct{ request, file, want int64 }{
		{64 << 20, 20 << 20, 64 << 20},
		{64 << 20, 300 << 20, 301 << 20},
		{8 << 20, 8 << 20, 9 << 20},
		{1, math.MaxInt64, math.MaxInt64},
	} {
		if got := Fit(tc.request, tc.file); got != tc.want {
			t.Fatalf("Fit(%d, %d) = %d, want %d", tc.request, tc.file, got, tc.want)
		}
	}
}
