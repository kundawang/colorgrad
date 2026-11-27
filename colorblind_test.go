package colorgrad

import (
	"math"
	"testing"
)

// approxEqual compares two floats with a relative tolerance.
func approxEqual(a, b, tol float64) bool {
	return math.Abs(a-b) <= tol*math.Max(1, math.Max(math.Abs(a), math.Abs(b)))
}

// Independent full LMS-pipeline deuteranopia simulation, used only in tests
// to cross-check the pre-multiplied matrix used by the library.
func refSimulateDeuteranopia(r, g, b float64) [3]float64 {
	rgb2lms := [3][3]float64{
		{17.8824, 43.5161, 4.11935},
		{3.45565, 27.1554, 3.86714},
		{0.0299566, 0.184309, 1.46709},
	}
	projection := [3][3]float64{
		{1, 0, 0},
		{0.494207, 0, 1.24827},
		{0, 0, 1},
	}
	lms2rgb := [3][3]float64{
		{0.0809444479, -0.130504409, 0.116721066},
		{-0.0102485335, 0.0540193266, -0.113614708},
		{-0.000365296938, -0.00412161469, 0.693511405},
	}
	apply := func(m [3][3]float64, v [3]float64) [3]float64 {
		var o [3]float64
		for i := 0; i < 3; i++ {
			o[i] = m[i][0]*v[0] + m[i][1]*v[1] + m[i][2]*v[2]
		}
		return o
	}
	lms := apply(rgb2lms, [3]float64{r, g, b})
	lms = apply(projection, lms)
	rgb := apply(lms2rgb, lms)
	for i := range rgb {
		rgb[i] = clamp01(rgb[i])
	}
	return rgb
}

// The pre-multiplied matrix must match the full LMS round trip of the
// published reference model.
func Test_ColorblindReferenceModel(t *testing.T) {
	samples := [][3]float64{
		{1, 0, 0}, // red
		{0, 1, 0}, // lime/green
		{0, 0, 1}, // blue
		{0, 0, 0}, // black
		{1, 1, 1}, // white
		{0.5, 0.5, 0.5},
		{0.75, 0.25, 0.4},
	}
	for _, s := range samples {
		got := simulateDeuteranopia(s[0], s[1], s[2])
		want := refSimulateDeuteranopia(s[0], s[1], s[2])
		for i := 0; i < 3; i++ {
			if !approxEqual(got[i], want[i], 1e-9) {
				t.Errorf("simulateDeuteranopia(%v)[%d] = %v, want %v", s, i, got[i], want[i])
			}
		}
	}

	// Published reference values for the primary colors.
	red := simulateDeuteranopia(1, 0, 0)
	testTrue(t, approxEqual(red[0], 0.29275078, 1e-6))
	testTrue(t, approxEqual(red[1], 0.29274970, 1e-6))
	testTrue(t, approxEqual(red[2], 0, 1e-6))

	lime := simulateDeuteranopia(0, 1, 0)
	testTrue(t, approxEqual(lime[0], 0.70725186, 1e-6))
	testTrue(t, approxEqual(lime[1], 0.70724921, 1e-6))
	testTrue(t, approxEqual(lime[2], 0.02233656, 1e-6))
}

// Fixed red-green swatches: deuteranopia collapses red and green onto nearly
// identical yellowish colors, so the check must report the pair.
func Test_ColorblindRedGreen(t *testing.T) {
	grad, err := NewGradient().HtmlColors("red", "lime").Build()
	test(t, err, nil)
	white := Rgb(1, 1, 1, 1)

	// Threshold 0.5: the red-green pair (distance ~0.356) is reported.
	conflicts, err := grad.ColorblindCheck(2, 0.5, white)
	test(t, err, nil)
	test(t, len(conflicts), 1)

	c := conflicts[0]
	test(t, c.Index, uint(0))
	test(t, c.Position1, 0.0)
	test(t, c.Position2, 1.0)
	test(t, c.Color1.HexString(), "#ff0000")
	test(t, c.Color2.HexString(), "#00ff00")
	testTrue(t, approxEqual(c.Distance, 0.3560438228, 1e-6))

	// A threshold above the distance still reports it.
	conflicts, err = grad.ColorblindCheck(2, 1.0, white)
	test(t, err, nil)
	test(t, len(conflicts), 1)

	// A threshold exactly equal to the distance does NOT report it:
	// the comparison is strictly "below threshold".
	conflicts, err = grad.ColorblindCheck(2, c.Distance, white)
	test(t, err, nil)
	test(t, len(conflicts), 0)

	// Sampling more colors yields deterministic, position-ordered reports.
	conflicts, err = grad.ColorblindCheck(5, 0.5, white)
	test(t, err, nil)
	test(t, len(conflicts), 4)
	for i, c := range conflicts {
		test(t, c.Index, uint(i))
		test(t, c.Position1, float64(i)/4)
		test(t, c.Position2, float64(i+1)/4)
	}
	// Positions strictly increase.
	for i := 1; i < len(conflicts); i++ {
		testTrue(t, conflicts[i-1].Position2 == conflicts[i].Position1)
		testTrue(t, conflicts[i].Position1 > conflicts[i-1].Position1)
	}
}

// Fixed black-white swatches: luminance is preserved, so the pair is never
// reported even at a fairly high threshold.
func Test_ColorblindBlackWhite(t *testing.T) {
	grad, err := NewGradient().HtmlColors("black", "white").Build()
	test(t, err, nil)
	white := Rgb(1, 1, 1, 1)

	conflicts, err := grad.ColorblindCheck(2, 0.5, white)
	test(t, err, nil)
	test(t, len(conflicts), 0)

	conflicts, err = grad.ColorblindCheck(2, 0.9, white)
	test(t, err, nil)
	test(t, len(conflicts), 0)

	// The full black-white perceptual distance is close to 1.
	all, err := grad.ColorblindCheck(2, 2.0, white)
	test(t, err, nil)
	test(t, len(all), 1)
	testTrue(t, approxEqual(all[0].Distance, 0.9999875967, 1e-6))

	// Dense sampling: only adjacent grayscale steps near black fall below 0.5,
	// and the report stays ordered.
	conflicts, err = grad.ColorblindCheck(10, 0.5, white)
	test(t, err, nil)
	test(t, len(conflicts), 9)
	for i, c := range conflicts {
		test(t, c.Index, uint(i))
	}
}

// Hard-edge gradients produce duplicate adjacent samples at segment
// boundaries; these must be reported with distance zero without panicking.
func Test_ColorblindHardEdge(t *testing.T) {
	base, err := NewGradient().HtmlColors("red", "lime").Build()
	test(t, err, nil)
	white := Rgb(1, 1, 1, 1)

	grad := base.Sharp(2, 0)
	conflicts, err := grad.ColorblindCheck(4, 0.5, white)
	test(t, err, nil)
	test(t, len(conflicts), 3)

	// First pair: identical red samples on either side of the hard edge.
	test(t, conflicts[0].Color1.HexString(), "#ff0000")
	test(t, conflicts[0].Color2.HexString(), "#ff0000")
	test(t, conflicts[0].Distance, 0.0)

	// Last pair: identical lime samples.
	test(t, conflicts[2].Color1.HexString(), "#00ff00")
	test(t, conflicts[2].Color2.HexString(), "#00ff00")
	test(t, conflicts[2].Distance, 0.0)

	// Middle pair: the red-green transition.
	test(t, conflicts[1].Color1.HexString(), "#ff0000")
	test(t, conflicts[1].Color2.HexString(), "#00ff00")
}

// Repeated colors yield zero distance and are always reported.
func Test_ColorblindDuplicateColors(t *testing.T) {
	grad, err := NewGradient().HtmlColors("red", "red").Build()
	test(t, err, nil)
	white := Rgb(1, 1, 1, 1)

	// Any positive threshold reports the zero-distance pair.
	conflicts, err := grad.ColorblindCheck(2, 1e-12, white)
	test(t, err, nil)
	test(t, len(conflicts), 1)
	test(t, conflicts[0].Distance, 0.0)
	test(t, conflicts[0].Color1.HexString(), "#ff0000")
	test(t, conflicts[0].Color2.HexString(), "#ff0000")

	// Threshold zero itself does not report: distance 0 is not strictly below 0.
	none, err := grad.ColorblindCheck(2, 0, white)
	test(t, err, nil)
	test(t, len(none), 0)
}

// Alpha is composited over the specified background before simulation.
func Test_ColorblindAlphaCompositing(t *testing.T) {
	white := Rgb(1, 1, 1, 1)
	black := Rgb(0, 0, 0, 1)

	// 50% transparent red over white equals opaque light red (#ff8080).
	grad, err := NewGradient().
		Colors(Rgb(1, 0, 0, 0.5), Rgb(1, 0.5, 0.5, 1)).
		Build()
	test(t, err, nil)

	conflicts, err := grad.ColorblindCheck(2, 2.0, white)
	test(t, err, nil)
	test(t, len(conflicts), 1)
	test(t, conflicts[0].Color1.HexString(), "#ff8080")
	test(t, conflicts[0].Color2.HexString(), "#ff8080")
	test(t, conflicts[0].Distance, 0.0)

	// Over black the same transparent color becomes dark red (#800000),
	// which is clearly distinguishable from the opaque light red.
	conflicts, err = grad.ColorblindCheck(2, 0.4, black)
	test(t, err, nil)
	test(t, len(conflicts), 0)
	all, err := grad.ColorblindCheck(2, 2.0, black)
	test(t, err, nil)
	test(t, all[0].Color1.HexString(), "#800000")
	testTrue(t, approxEqual(all[0].Distance, 0.4536439816, 1e-6))

	// Fully transparent color composites to the background itself.
	grad2, err := NewGradient().
		Colors(Rgb(0.2, 0.8, 0.1, 0), Rgb(0.2, 0.8, 0.1, 0)).
		Build()
	test(t, err, nil)
	conflicts, err = grad2.ColorblindCheck(2, 2.0, white)
	test(t, err, nil)
	test(t, len(conflicts), 1)
	test(t, conflicts[0].Color1.HexString(), "#ffffff")
	test(t, conflicts[0].Distance, 0.0)
}

// Invalid arguments produce errors and never panic.
func Test_ColorblindInvalidArguments(t *testing.T) {
	grad, err := NewGradient().HtmlColors("red", "lime").Build()
	test(t, err, nil)
	white := Rgb(1, 1, 1, 1)

	if _, err := grad.ColorblindCheck(0, 1, white); err == nil {
		t.Error("count 0 should return an error")
	}
	if _, err := grad.ColorblindCheck(1, 1, white); err == nil {
		t.Error("count 1 should return an error")
	}
	if _, err := grad.ColorblindCheck(2, -0.1, white); err == nil {
		t.Error("negative threshold should return an error")
	}
	if _, err := grad.ColorblindCheck(2, math.NaN(), white); err == nil {
		t.Error("NaN threshold should return an error")
	}
	if _, err := grad.ColorblindCheck(2, math.Inf(1), white); err == nil {
		t.Error("+Inf threshold should return an error")
	}
	if _, err := grad.ColorblindCheck(2, math.Inf(-1), white); err == nil {
		t.Error("-Inf threshold should return an error")
	}
	if _, err := grad.ColorblindCheck(2, 1, Rgb(math.NaN(), 1, 1, 1)); err == nil {
		t.Error("NaN background should return an error")
	}
	if _, err := grad.ColorblindCheck(2, 1, Rgb(1, math.Inf(1), 1, 1)); err == nil {
		t.Error("Inf background should return an error")
	}

	// Non-finite sampled color.
	bad, err := NewGradient().Colors(Rgb(math.NaN(), 0, 0, 1), Rgb(0, 1, 0, 1)).Build()
	test(t, err, nil)
	if _, err := bad.ColorblindCheck(2, 1, white); err == nil {
		t.Error("non-finite sampled color should return an error")
	}
	bad2, err := NewGradient().Colors(Rgb(1, 0, 0, 1), Rgb(0, math.Inf(1), 0, 1)).Build()
	test(t, err, nil)
	if _, err := bad2.ColorblindCheck(2, 1, white); err == nil {
		t.Error("Inf sampled color should return an error")
	}
}

// Repeated calls return identical, position-ordered results and leave the
// original gradient untouched.
func Test_ColorblindDeterministicAndImmutable(t *testing.T) {
	grad, err := NewGradient().HtmlColors("red", "lime", "blue").Build()
	test(t, err, nil)
	white := Rgb(1, 1, 1, 1)

	first, err := grad.ColorblindCheck(8, 0.5, white)
	test(t, err, nil)
	for run := 0; run < 3; run++ {
		next, err := grad.ColorblindCheck(8, 0.5, white)
		test(t, err, nil)
		test(t, len(next), len(first))
		for i := range first {
			test(t, next[i], first[i])
		}
	}

	// Empty (but non-nil) result when nothing is below threshold.
	none, err := grad.ColorblindCheck(2, 0, white)
	test(t, err, nil)
	test(t, len(none), 0)
	testTrue(t, none != nil)

	// The gradient output is unchanged after the check.
	testSlice(t, colors2hex(grad.Colors(3)), []string{"#ff0000", "#00ff00", "#0000ff"})
	test(t, grad.At(0).HexString(), "#ff0000")
	test(t, grad.At(1).HexString(), "#0000ff")
}
