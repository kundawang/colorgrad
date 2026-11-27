package colorgrad

import (
	"math"
	"testing"
)

func approxColor(a, b Color) bool {
	const tol = 1e-12
	return math.Abs(a.R-b.R) <= tol &&
		math.Abs(a.G-b.G) <= tol &&
		math.Abs(a.B-b.B) <= tol &&
		math.Abs(a.A-b.A) <= tol
}

func testColor(t *testing.T, got, want Color) {
	t.Helper()
	if !approxColor(got, want) {
		t.Errorf("color mismatch -> got: %+v, want: %+v", got, want)
	}
}

// Fixed 2x2 four-corner grid. Also covers non-default domain, exact corners,
// center and edge interpolation, border clamping and non-finite queries.
func Test_GridGradient_2x2(t *testing.T) {
	red := Rgb(1, 0, 0, 1)
	green := Rgb(0, 1, 0, 1)
	blue := Rgb(0, 0, 1, 1)
	white := Rgb(1, 1, 1, 1)

	// xs and ys deliberately differ in span and offset.
	grad, err := NewGridGradient(
		[]float64{0, 10},
		[]float64{-1, 1},
		[][]Color{
			{red, green},   // y = -1
			{blue, white},  // y = 1
		},
		BlendRgb,
	)
	test(t, err, nil)

	xmin, xmax, ymin, ymax := grad.Domain()
	test(t, xmin, 0.0)
	test(t, xmax, 10.0)
	test(t, ymin, -1.0)
	test(t, ymax, 1.0)

	// The four corners must be exact.
	test(t, grad.At(0, -1), red)
	test(t, grad.At(10, -1), green)
	test(t, grad.At(0, 1), blue)
	test(t, grad.At(10, 1), white)

	// Center: bilinear average of the four corners.
	testColor(t, grad.At(5, 0), Rgb(0.5, 0.5, 0.5, 1))

	// Edge midpoints.
	testColor(t, grad.At(5, -1), Rgb(0.5, 0.5, 0, 1))
	testColor(t, grad.At(5, 1), Rgb(0.5, 0.5, 1, 1))
	testColor(t, grad.At(0, 0), Rgb(0.5, 0, 0.5, 1))
	testColor(t, grad.At(10, 0), Rgb(0.5, 1, 0.5, 1))

	// Positions outside the domain are clamped to the borders.
	test(t, grad.At(-100, -100), red)
	test(t, grad.At(100, 100), white)
	testColor(t, grad.At(5, -100), Rgb(0.5, 0.5, 0, 1))
	testColor(t, grad.At(100, 0), Rgb(0.5, 1, 0.5, 1))
	testColor(t, grad.At(-100, 0), Rgb(0.5, 0, 0.5, 1))

	// Infinities clamp like ordinary out-of-range positions.
	testColor(t, grad.At(math.Inf(1), 0), Rgb(0.5, 1, 0.5, 1))
	testColor(t, grad.At(math.Inf(-1), 0), Rgb(0.5, 0, 0.5, 1))
	test(t, grad.At(0, math.Inf(1)), blue)
	test(t, grad.At(0, math.Inf(-1)), red)

	// NaN coordinates produce a deterministic result.
	test(t, grad.At(math.NaN(), 0), Color{A: 1})
	test(t, grad.At(0, math.NaN()), Color{A: 1})
}

// Non-uniformly spaced stops with multiple cells: the local parameter must be
// normalized within each cell, not over the whole domain.
func Test_GridGradient_NonUniform(t *testing.T) {
	red := Rgb(1, 0, 0, 1)
	green := Rgb(0, 1, 0, 1)
	blue := Rgb(0, 0, 1, 1)
	black := Rgb(0, 0, 0, 1)
	yellow := Rgb(1, 1, 0, 1)
	white := Rgb(1, 1, 1, 1)

	grad, err := NewGridGradient(
		[]float64{0, 1, 10},
		[]float64{0, 5},
		[][]Color{
			{red, green, blue},    // y = 0
			{black, yellow, white}, // y = 5
		},
		BlendRgb,
	)
	test(t, err, nil)

	// Every grid knot must be exact.
	test(t, grad.At(0, 0), red)
	test(t, grad.At(1, 0), green)
	test(t, grad.At(10, 0), blue)
	test(t, grad.At(0, 5), black)
	test(t, grad.At(1, 5), yellow)
	test(t, grad.At(10, 5), white)

	// Midpoint of the narrow first cell.
	testColor(t, grad.At(0.5, 0), Rgb(0.5, 0.5, 0, 1))

	// Midpoint of the wide second cell, whose local half is x = 5.5.
	testColor(t, grad.At(5.5, 0), Rgb(0, 0.5, 0.5, 1))

	// Interior knot at half height: average of green and yellow.
	testColor(t, grad.At(1, 2.5), Rgb(0.5, 1, 0, 1))

	// Non-uniform normalization at 1/9 of the wide cell on the top edge.
	testColor(t, grad.At(2, 5), Rgb(1, 1, 1.0/9.0, 1))
}

// Transparent corners: alpha takes part in the interpolation and fully
// transparent control colors remain well defined.
func Test_GridGradient_Alpha(t *testing.T) {
	c00 := Rgb(1, 0, 0, 0)
	c10 := Rgb(0, 1, 0, 1)
	c01 := Rgb(0, 0, 1, 1)
	c11 := Rgb(1, 1, 1, 0)

	grad, err := NewGridGradient(
		[]float64{0, 1},
		[]float64{0, 1},
		[][]Color{
			{c00, c10},
			{c01, c11},
		},
		BlendRgb,
	)
	test(t, err, nil)

	// Exact transparent corners, including zero alpha.
	test(t, grad.At(0, 0), c00)
	test(t, grad.At(1, 1), c11)

	// Bilinear interpolation of both channels and alpha.
	testColor(t, grad.At(0.5, 0.5), Rgb(0.5, 0.5, 0.5, 0.5))
	testColor(t, grad.At(0.5, 0), Rgb(0.5, 0.5, 0, 0.5))
	testColor(t, grad.At(0, 0.5), Rgb(0.5, 0, 0.5, 0.5))}

// All blend modes share the same grid semantics; corners stay exact and alpha
// interpolation is mode independent.
func Test_GridGradient_Modes(t *testing.T) {
	c00 := Rgb(1, 0, 0, 0.2)
	c10 := Rgb(0, 1, 0, 0.4)
	c01 := Rgb(0, 0, 1, 0.8)
	c11 := Rgb(1, 1, 1, 1)
	grid := [][]Color{{c00, c10}, {c01, c11}}

	for _, mode := range []BlendMode{BlendRgb, BlendLinearRgb, BlendLab, BlendOklab} {
		grad, err := NewGridGradient([]float64{0, 1}, []float64{0, 1}, grid, mode)
		test(t, err, nil)

		// Exact control points in every mode.
		test(t, grad.At(0, 0), c00)
		test(t, grad.At(1, 0), c10)
		test(t, grad.At(0, 1), c01)
		test(t, grad.At(1, 1), c11)

		got := grad.At(0.5, 0.5)
		if !isFinite(got.R) || !isFinite(got.G) || !isFinite(got.B) || !isFinite(got.A) {
			t.Errorf("%s: non-finite interpolated color: %+v", mode, got)
		}
		// Center alpha is the mean of the four corner alphas.
		if math.Abs(got.A-0.6) > 1e-12 {
			t.Errorf("%s: alpha mismatch -> got: %v, want: 0.6", mode, got.A)
		}
	}

	_, err := NewGridGradient([]float64{0, 1}, []float64{0, 1}, grid, BlendMode(42))
	testTrue(t, err != nil)
}

func Test_GridGradient_Errors(t *testing.T) {
	grid := [][]Color{
		{Rgb(1, 0, 0, 1), Rgb(0, 1, 0, 1)},
		{Rgb(0, 0, 1, 1), Rgb(1, 1, 1, 1)},
	}

	type tc struct {
		name   string
		xs     []float64
		ys     []float64
		colors [][]Color
		mode   BlendMode
	}
	cases := []tc{
		{"xs too short", []float64{0}, []float64{0, 1}, grid, BlendRgb},
		{"ys too short", []float64{0, 1}, []float64{0}, grid, BlendRgb},
		{"duplicate xs", []float64{0, 0}, []float64{0, 1}, grid, BlendRgb},
		{"duplicate ys", []float64{0, 1}, []float64{1, 1}, grid, BlendRgb},
		{"unsorted xs", []float64{1, 0}, []float64{0, 1}, grid, BlendRgb},
		{"unsorted ys", []float64{0, 1}, []float64{2, 1}, grid, BlendRgb},
		{"nan xs", []float64{math.NaN(), 1}, []float64{0, 1}, grid, BlendRgb},
		{"inf xs", []float64{0, math.Inf(1)}, []float64{0, 1}, grid, BlendRgb},
		{"nan ys", []float64{0, 1}, []float64{0, math.NaN()}, grid, BlendRgb},
		{"row count mismatch", []float64{0, 1}, []float64{0, 1, 2}, grid, BlendRgb},
		{"row length mismatch", []float64{0, 1, 2}, []float64{0, 1},
			[][]Color{{Rgb(0, 0, 0, 1)}, {Rgb(1, 1, 1, 1)}}, BlendRgb},
		{"non-finite color", []float64{0, 1}, []float64{0, 1},
			[][]Color{
				{Rgb(math.NaN(), 0, 0, 1), Rgb(0, 1, 0, 1)},
				{Rgb(0, 0, 1, 1), Rgb(1, 1, 1, 1)},
			}, BlendRgb},
		{"invalid mode", []float64{0, 1}, []float64{0, 1}, grid, BlendMode(99)},
	}
	for _, c := range cases {
		if _, err := NewGridGradient(c.xs, c.ys, c.colors, c.mode); err == nil {
			t.Errorf("%s: expected error, got nil", c.name)
		}
	}
}

// Input slices are copied: mutating them after construction must not change
// the gradient, and failed construction yields a usable zero value.
func Test_GridGradient_DefensiveCopyAndZero(t *testing.T) {
	xs := []float64{0, 1}
	ys := []float64{0, 1}
	grid := [][]Color{
		{Rgb(1, 0, 0, 1), Rgb(0, 1, 0, 1)},
		{Rgb(0, 0, 1, 1), Rgb(1, 1, 1, 1)},
	}
	grad, err := NewGridGradient(xs, ys, grid, BlendRgb)
	test(t, err, nil)

	want := grad.At(0.5, 0.5)
	xs[0] = -999
	ys[1] = 999
	grid[0][0] = Rgb(0, 0, 0, 0)
	testColor(t, grad.At(0.5, 0.5), want)
	test(t, grad.At(0, 0), Rgb(1, 0, 0, 1))

	// Zero value is safe to query.
	var zero GridGradient
	test(t, zero.At(0, 0), Color{})
	xmin, xmax, ymin, ymax := zero.Domain()
	test(t, xmin, 0.0)
	test(t, xmax, 0.0)
	test(t, ymin, 0.0)
	test(t, ymax, 0.0)

	// A constructor error also returns a safe zero value.
	bad, err := NewGridGradient([]float64{0}, []float64{0, 1}, grid, BlendRgb)
	testTrue(t, err != nil)
	test(t, bad.At(0, 0), Color{})
}
