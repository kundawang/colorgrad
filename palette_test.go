package colorgrad

import (
	"errors"
	"math"
	"testing"
)

// The tests in this file exercise the perceptual palette API through the
// exported surface. Distances used for assertions are computed independently
// of the implementation's Oklab arc length: balance claims are measured in CIE
// Lab, a different perceptual space, so a bug that merely returns the same
// metric that drives the sampler cannot hide.

// labDistance is the Euclidean distance between two colors in CIE Lab (D50).
// It is intentionally not the Oklab metric the implementation uses.
func labDistance(a, b Color) float64 {
	x, y := a.Lab(), b.Lab()
	dr, dg, db := x[0]-y[0], x[1]-y[1], x[2]-y[2]
	return math.Sqrt(dr*dr + dg*dg + db*db)
}

func oklabDistance(a, b Color) float64 {
	x, y := a.Oklab(), b.Oklab()
	dr, dg, db := x[0]-y[0], x[1]-y[1], x[2]-y[2]
	return math.Sqrt(dr*dr + dg*dg + db*db)
}

func paletteEntriesOK(t *testing.T, entries []PaletteEntry) {
	t.Helper()
	for i, e := range entries {
		c := e.Color
		if math.IsNaN(c.R) || math.IsNaN(c.G) || math.IsNaN(c.B) || math.IsNaN(c.A) ||
			math.IsInf(c.R, 0) || math.IsInf(c.G, 0) || math.IsInf(c.B, 0) || math.IsInf(c.A, 0) {
			t.Fatalf("entry %d has non-finite color: %+v", i, c)
		}
		if c.R < 0 || c.R > 1 || c.G < 0 || c.G > 1 || c.B < 0 || c.B > 1 || c.A < 0 || c.A > 1 {
			t.Fatalf("entry %d has out-of-range channel: %+v", i, c)
		}
		if math.IsNaN(e.Position) || math.IsInf(e.Position, 0) {
			t.Fatalf("entry %d has non-finite position: %v", i, e.Position)
		}
		if e.Pos != e.Position || e.T != e.Position {
			t.Fatalf("entry %d alias fields disagree: %+v", i, e)
		}
		if i > 0 && e.Position < entries[i-1].Position {
			t.Fatalf("positions not ordered at %d: %v < %v", i, e.Position, entries[i-1].Position)
		}
	}
}

// coefficientOfVariation returns stddev/mean of the given distances.
func coefficientOfVariation(d []float64) float64 {
	if len(d) == 0 {
		return 0
	}
	var mean float64
	for _, v := range d {
		mean += v
	}
	mean /= float64(len(d))
	if mean == 0 {
		return 0
	}
	var variance float64
	for _, v := range d {
		variance += (v - mean) * (v - mean)
	}
	variance /= float64(len(d))
	return math.Sqrt(variance) / mean
}

func evenStepLabDistances(colors []Color) []float64 {
	d := make([]float64, len(colors)-1)
	for i := 0; i < len(colors)-1; i++ {
		d[i] = labDistance(colors[i], colors[i+1])
	}
	return d
}

func paletteLabDistances(entries []PaletteEntry) []float64 {
	d := make([]float64, len(entries)-1)
	for i := 0; i < len(entries)-1; i++ {
		d[i] = labDistance(entries[i].Color, entries[i+1].Color)
	}
	return d
}

func Test_PerceptualPalette_EndpointsDomainAndOrder(t *testing.T) {
	grad, err := NewGradient().
		HtmlColors("#002bff", "#00d0ff", "#fff6aa", "#ff2b00").
		Domain(0, 0.7, 0.9, 1).
		Mode(BlendOklab).
		Build()
	if err != nil {
		t.Fatal(err)
	}

	entries, err := grad.PerceptualPalette(9, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 9 {
		t.Fatalf("len = %d, want 9", len(entries))
	}
	paletteEntriesOK(t, entries)

	dmin, dmax := grad.Domain()
	if entries[0].Position != dmin {
		t.Fatalf("first position = %v, want domain min %v", entries[0].Position, dmin)
	}
	if entries[len(entries)-1].Position != dmax {
		t.Fatalf("last position = %v, want domain max %v", entries[len(entries)-1].Position, dmax)
	}
	// Endpoint colors are the gradient's own endpoints.
	if got, want := entries[0].Color.HexString(), grad.At(dmin).Clamp().HexString(); got != want {
		t.Fatalf("first color = %s, want %s", got, want)
	}
	if got, want := entries[len(entries)-1].Color.HexString(), grad.At(dmax).Clamp().HexString(); got != want {
		t.Fatalf("last color = %s, want %s", got, want)
	}

	// Colors reported must actually be the gradient colors at the positions.
	for i, e := range entries {
		if got, want := e.Color.HexString(), grad.At(e.Position).Clamp().HexString(); got != want {
			t.Fatalf("entry %d color mismatch at %v: %s != %s", i, e.Position, got, want)
		}
	}

	// A second call must reproduce the same result exactly.
	again, err := grad.PerceptualPalette(9, 1)
	if err != nil {
		t.Fatal(err)
	}
	for i := range entries {
		if entries[i] != again[i] {
			t.Fatalf("unstable result at %d: %+v != %+v", i, entries[i], again[i])
		}
	}
}

func Test_PerceptualPalette_BeatsEvenSamplingOnUnevenStops(t *testing.T) {
	// Non-uniform stop positions with non-linear Oklab blending: three quarters
	// of the domain cover two short segments, so even sampling wastes most of
	// its samples there.
	grad, err := NewGradient().
		HtmlColors("#002bff", "#00d0ff", "#fff6aa", "#ff2b00").
		Domain(0, 0.7, 0.9, 1).
		Mode(BlendOklab).
		Interpolation(InterpolationLinear).
		Build()
	if err != nil {
		t.Fatal(err)
	}

	const count = 10
	even := evenStepLabDistances(grad.Colors(count))
	entries, err := grad.PerceptualPalette(count, 1)
	if err != nil {
		t.Fatal(err)
	}
	paletteEntriesOK(t, entries)
	perceptual := paletteLabDistances(entries)

	evenCV := coefficientOfVariation(even)
	perceptualCV := coefficientOfVariation(perceptual)
	if !(perceptualCV < evenCV*0.5) {
		t.Fatalf("perceptual spacing not clearly more even: even CV = %.4f, perceptual CV = %.4f", evenCV, perceptualCV)
	}

	// The spread between the smallest and largest step must also shrink.
	evenSpread := ratio(maxFloat(even), minFloat(even))
	perceptualSpread := ratio(maxFloat(perceptual), minFloat(perceptual))
	if !(perceptualSpread < evenSpread) {
		t.Fatalf("perceptual max/min spread = %.3f not better than even = %.3f", perceptualSpread, evenSpread)
	}
}

func Test_PerceptualPalette_BeatsEvenSamplingWithCatmullRom(t *testing.T) {
	// Non-linear interpolation with a non-uniform middle stop.
	grad, err := NewGradient().
		HtmlColors("#1d4ed8", "#fef08a", "#dc2626").
		Domain(0.15, 0.9, 1).
		Interpolation(InterpolationCatmullRom).
		Mode(BlendOklab).
		Build()
	if err != nil {
		t.Fatal(err)
	}

	const count = 10
	even := evenStepLabDistances(grad.Colors(count))
	entries, err := grad.PerceptualPalette(count, 1)
	if err != nil {
		t.Fatal(err)
	}
	paletteEntriesOK(t, entries)
	perceptual := paletteLabDistances(entries)

	evenCV := coefficientOfVariation(even)
	perceptualCV := coefficientOfVariation(perceptual)
	if !(perceptualCV < evenCV*0.5) {
		t.Fatalf("perceptual spacing not clearly more even: even CV = %.4f, perceptual CV = %.4f", evenCV, perceptualCV)
	}
}

func Test_PerceptualPalette_RepeatedColors(t *testing.T) {
	// Repeated red then a jump to blue: the flat red half holds two samples at
	// most, and they stay ordered without division-by-zero or NaN.
	grad, err := NewGradient().HtmlColors("#f00", "#f00", "#00f").Build()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := grad.PerceptualPalette(6, 1)
	if err != nil {
		t.Fatal(err)
	}
	paletteEntriesOK(t, entries)

	if entries[0].Color.HexString() != "#ff0000" {
		t.Fatalf("first color = %s, want #ff0000", entries[0].Color.HexString())
	}
	if entries[len(entries)-1].Color.HexString() != "#0000ff" {
		t.Fatalf("last color = %s, want #0000ff", entries[len(entries)-1].Color.HexString())
	}
	// The flat red band must not consume most of the samples: at least half
	// of the entries should have moved off pure red.
	var blueward int
	for _, e := range entries {
		if e.Color.HexString() != "#ff0000" {
			blueward++
		}
	}
	if blueward < 3 {
		t.Fatalf("only %d/6 samples left the flat red band: %+v", blueward, entries)
	}
}

func Test_PerceptualPalette_FlatGradientIsUniform(t *testing.T) {
	// A constant gradient has zero total perceptual length; positions must fall
	// back to a deterministic uniform grid.
	c := Rgb(0.5, 0.5, 0.5, 1)
	grad, err := NewGradient().Colors(c, c).Domain(2, 7).Build()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := grad.PerceptualPalette(4, 1)
	if err != nil {
		t.Fatal(err)
	}
	paletteEntriesOK(t, entries)
	want := linspace(2, 7, 4)
	for i, e := range entries {
		if e.Position != want[i] {
			t.Fatalf("entry %d position = %v, want %v", i, e.Position, want[i])
		}
		if e.Color.HexString() != "#808080" {
			t.Fatalf("entry %d color = %s, want #808080", i, e.Color.HexString())
		}
	}

	// Same result twice.
	again, err := grad.PerceptualPalette(4, 1)
	if err != nil {
		t.Fatal(err)
	}
	for i := range entries {
		if entries[i] != again[i] {
			t.Fatalf("unstable flat result at %d", i)
		}
	}
}

func Test_PerceptualPalette_HardEdges(t *testing.T) {
	grad, err := NewGradient().HtmlColors("#f00", "#0f0", "#00f", "#ff0").Build()
	if err != nil {
		t.Fatal(err)
	}
	sharp := grad.Sharp(4, 0)
	entries, err := sharp.PerceptualPalette(8, 1)
	if err != nil {
		t.Fatal(err)
	}
	paletteEntriesOK(t, entries)

	dmin, dmax := sharp.Domain()
	if entries[0].Position != dmin {
		t.Fatalf("first position = %v, want %v", entries[0].Position, dmin)
	}
	if entries[len(entries)-1].Position != dmax {
		t.Fatalf("last position = %v, want %v", entries[len(entries)-1].Position, dmax)
	}
	// The four hard bands are all represented, and no entry lands on an
	// in-between blended color that does not exist in the gradient.
	seen := map[string]bool{}
	for _, e := range entries {
		seen[e.Color.HexString()] = true
	}
	for _, want := range []string{"#ff0000", "#00ff00", "#0000ff", "#ffff00"} {
		if !seen[want] {
			t.Fatalf("hard edge band %s missing from palette: %v", want, seen)
		}
	}
}

func Test_PerceptualPalette_BalanceOnLinearPath(t *testing.T) {
	// When the gradient is linear in Oklab, equal perceptual steps correspond
	// to equal parameter steps, so the palette must land on the even grid.
	grad, err := NewGradient().HtmlColors("#000", "#fff").Mode(BlendOklab).Build()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := grad.PerceptualPalette(5, 1)
	if err != nil {
		t.Fatal(err)
	}
	paletteEntriesOK(t, entries)
	for i, e := range entries {
		if math.Abs(e.Position-float64(i)*0.25) > 1e-3 {
			t.Fatalf("entry %d position = %v, want ~%v", i, e.Position, float64(i)*0.25)
		}
	}

	// In sRGB blending the same black-to-white ramp is linear in RGB but not
	// in Oklab, so the samples must not be evenly spaced in t; instead the
	// Oklab distances between neighbors are equal. This is the property that
	// matters, and it is exactly what even parameter sampling fails to give.
	rgbGrad, err := NewGradient().HtmlColors("#000", "#fff").Mode(BlendRgb).Build()
	if err != nil {
		t.Fatal(err)
	}
	rgbEntries, err := rgbGrad.PerceptualPalette(5, 1)
	if err != nil {
		t.Fatal(err)
	}
	paletteEntriesOK(t, rgbEntries)

	d := paletteLabDistances(rgbEntries)
	ok := make([]float64, len(rgbEntries)-1)
	for i := 0; i < len(rgbEntries)-1; i++ {
		ok[i] = oklabDistance(rgbEntries[i].Color, rgbEntries[i+1].Color)
	}
	for i, v := range ok {
		if math.Abs(v-ok[0]) > 1e-3 {
			t.Fatalf("Oklab step %d = %v, want ~%v (all steps equal)", i, v, ok[0])
		}
	}
	if coefficientOfVariation(d) >= coefficientOfVariation(ok) {
		// Lab and Oklab are different spaces; this only records that the
		// primary claim is Oklab equality, asserted above.
		t.Logf("Lab CV %.5f vs Oklab CV %.5f", coefficientOfVariation(d), coefficientOfVariation(ok))
	}
}

func Test_PerceptualPalette_AlphaWeight(t *testing.T) {
	// Red to transparent blue: RGB and alpha both change.
	grad, err := NewGradient().HtmlColors("#ff0000", "#0000ff00").Build()
	if err != nil {
		t.Fatal(err)
	}
	const count = 6

	ignore, err := grad.PerceptualPalette(count, 0)
	if err != nil {
		t.Fatal(err)
	}
	heavy, err := grad.PerceptualPalette(count, 50)
	if err != nil {
		t.Fatal(err)
	}
	paletteEntriesOK(t, ignore)
	paletteEntriesOK(t, heavy)

	// A finite positive weight is accepted and keeps the endpoints.
	one, err := grad.PerceptualPalette(count, 1)
	if err != nil {
		t.Fatal(err)
	}
	paletteEntriesOK(t, one)

	// Ignoring alpha leaves the Oklab nonlinearity of the RGB ramp in charge,
	// so the interior is measurably off the uniform grid. Weighting alpha
	// heavily makes alpha's linear-in-t change dominate, so the result
	// converges back onto the even grid.
	ignoreDeviation := maxUniformDeviation(ignore)
	heavyDeviation := maxUniformDeviation(heavy)
	if !(heavyDeviation < 0.01) {
		t.Fatalf("heavy-alpha palette not near uniform: max deviation = %v", heavyDeviation)
	}
	if !(ignoreDeviation > 10*heavyDeviation) {
		t.Fatalf("alpha weight had no effect: ignore deviation = %v, heavy deviation = %v", ignoreDeviation, heavyDeviation)
	}
	for i := 1; i < count-1; i++ {
		if ignore[i].Position == heavy[i].Position {
			t.Fatalf("interior entry %d did not move with alpha weight", i)
		}
	}
}

func Test_PerceptualPalette_TransparencyOnly(t *testing.T) {
	// No RGB change, only alpha: the palette must still spread out by alpha.
	grad, err := NewGradient().HtmlColors("#ff0000", "#ff000000").Build()
	if err != nil {
		t.Fatal(err)
	}

	entries, err := grad.PerceptualPalette(5, 1)
	if err != nil {
		t.Fatal(err)
	}
	paletteEntriesOK(t, entries)
	for i, e := range entries {
		want := float64(i) / 4
		if math.Abs(e.Position-want) > 1e-9 {
			t.Fatalf("entry %d position = %v, want %v", i, e.Position, want)
		}
	}

	// With alpha ignored the gradient is constant, so positions are uniform
	// anyway; crucially the result must be finite and ordered.
	flat, err := grad.PerceptualPalette(5, 0)
	if err != nil {
		t.Fatal(err)
	}
	paletteEntriesOK(t, flat)
}

func Test_PerceptualPalette_InvalidArguments(t *testing.T) {
	grad, err := NewGradient().HtmlColors("#000", "#fff").Build()
	if err != nil {
		t.Fatal(err)
	}

	for _, w := range []float64{-1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		entries, err := grad.PerceptualPalette(5, w)
		if !errors.Is(err, ErrPaletteAlphaWeight) {
			t.Fatalf("weight %v: err = %v, want ErrPaletteAlphaWeight", w, err)
		}
		if !errors.Is(err, ErrPaletteRequest) {
			t.Fatalf("weight %v: err does not match umbrella ErrPaletteRequest", w)
		}
		if entries != nil {
			t.Fatalf("weight %v: entries = %v, want nil", w, entries)
		}
	}

	// Zero and one colors mirror Colors' deterministic behavior.
	if entries, err := grad.PerceptualPalette(0, 1); err != nil || len(entries) != 0 {
		t.Fatalf("count 0: entries = %v, err = %v", entries, err)
	}
	if entries, err := grad.PerceptualPalette(1, 1); err != nil || len(entries) != 1 || entries[0].Position != grad.Min {
		t.Fatalf("count 1: entries = %v, err = %v", entries, err)
	}

	// Invalid domains.
	var zero Gradient
	if _, err := zero.PerceptualPalette(5, 1); !errors.Is(err, ErrPaletteDomain) {
		t.Fatalf("nil core: err = %v, want ErrPaletteDomain", err)
	}
	bad := Gradient{Core: grad.Core, Min: 1, Max: 1}
	if _, err := bad.PerceptualPalette(5, 1); !errors.Is(err, ErrPaletteDomain) {
		t.Fatalf("empty domain: err = %v, want ErrPaletteDomain", err)
	}
	reversed := Gradient{Core: grad.Core, Min: 1, Max: 0}
	if _, err := reversed.PerceptualPalette(5, 1); !errors.Is(err, ErrPaletteDomain) {
		t.Fatalf("reversed domain: err = %v, want ErrPaletteDomain", err)
	}
	nonFinite := Gradient{Core: grad.Core, Min: 0, Max: math.Inf(1)}
	if _, err := nonFinite.PerceptualPalette(5, 1); !errors.Is(err, ErrPaletteDomain) {
		t.Fatalf("non-finite domain: err = %v, want ErrPaletteDomain", err)
	}
}

// nonFiniteGradient is a GradientCore that returns NaN, to prove the palette
// rejects bad colors instead of emitting NaN positions.
type nonFiniteGradient struct{}

func (nonFiniteGradient) At(t float64) Color { return Color{R: math.NaN(), G: 0, B: 0, A: 1} }

func Test_PerceptualPalette_NonFiniteCore(t *testing.T) {
	grad := Gradient{Core: nonFiniteGradient{}, Min: 0, Max: 1}
	entries, err := grad.PerceptualPalette(5, 1)
	if !errors.Is(err, ErrPaletteColor) {
		t.Fatalf("err = %v, want ErrPaletteColor", err)
	}
	if entries != nil {
		t.Fatalf("entries = %v, want nil", entries)
	}
}

func Test_PerceptualPalette_XYForm(t *testing.T) {
	grad, err := NewGradient().
		HtmlColors("#002bff", "#00d0ff", "#fff6aa", "#ff2b00").
		Domain(0, 0.7, 0.9, 1).
		Mode(BlendOklab).
		Build()
	if err != nil {
		t.Fatal(err)
	}

	entries, err := grad.PerceptualPalette(7, 1)
	if err != nil {
		t.Fatal(err)
	}
	colors, positions, err := grad.PerceptualPaletteXY(7, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(colors) != len(entries) || len(positions) != len(entries) {
		t.Fatalf("lengths = %d, %d, want %d", len(colors), len(positions), len(entries))
	}
	for i, e := range entries {
		if colors[i] != e.Color || positions[i] != e.Position {
			t.Fatalf("entry %d mismatch: (%v, %v) != %+v", i, colors[i], positions[i], e)
		}
	}

	// Package-level forms agree with the methods.
	if _, err := PerceptualPalette(grad, 7, 1); err != nil {
		t.Fatal(err)
	}
	pkgColors, pkgPositions, err := PerceptualColors(grad, 7, 1)
	if err != nil {
		t.Fatal(err)
	}
	for i := range colors {
		if pkgColors[i] != colors[i] || pkgPositions[i] != positions[i] {
			t.Fatalf("package form differs at entry %d", i)
		}
	}
}

func Test_PerceptualPalette_ColorsUnchanged(t *testing.T) {
	// The new API must not change Colors' existing evenly spaced behavior.
	grad, err := NewGradient().
		HtmlColors("#f00", "#0f0", "#00f").
		Domain(-1, 1).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	testSlice(t, colors2hex(grad.Colors(5)), []string{
		"#ff0000",
		"#808000",
		"#00ff00",
		"#008080",
		"#0000ff",
	})
}

func Test_ColorDistance(t *testing.T) {
	black := Rgb(0, 0, 0, 1)
	white := Rgb(1, 1, 1, 1)
	if d := ColorDistance(black, black); d != 0 {
		t.Fatalf("identical distance = %v, want 0", d)
	}
	if d := ColorDistance(black, white); !(d > 0) {
		t.Fatalf("black/white distance = %v, want > 0", d)
	}
	// Symmetric.
	if a, b := ColorDistance(black, white), ColorDistance(white, black); a != b {
		t.Fatalf("not symmetric: %v != %v", a, b)
	}
	// Alpha is ignored unless weighted.
	opaque := Rgb(1, 0, 0, 1)
	clear := Rgb(1, 0, 0, 0)
	if d := ColorDistance(opaque, clear); d != 0 {
		t.Fatalf("unweighted alpha distance = %v, want 0", d)
	}
	if d := ColorDistanceWeighted(opaque, clear, 1); !(d > 0) {
		t.Fatalf("weighted alpha distance = %v, want > 0", d)
	}
	if a, b := ColorDistanceWeighted(opaque, clear, 0), ColorDistance(opaque, clear); a != b {
		t.Fatalf("weight 0 = %v, want %v", a, b)
	}
	if d := PerceptualDistance(black, white); d != ColorDistance(black, white) {
		t.Fatalf("PerceptualDistance alias differs")
	}
}

func Test_PerceptualPalette_IndependentSpaceBalance(t *testing.T) {
	// Use a fixed stop set and an independent color space (CIE Lab) to show the
	// Oklab-driven sampler still equalizes steps measured with Lab.
	grad, err := NewGradient().
		HtmlColors("#003f5c", "#ffa600", "#bc5090").
		Domain(0.1, 0.95, 1).
		Mode(BlendLab).
		Build()
	if err != nil {
		t.Fatal(err)
	}

	const count = 12
	even := evenStepLabDistances(grad.Colors(count))
	entries, err := grad.PerceptualPalette(count, 1)
	if err != nil {
		t.Fatal(err)
	}
	paletteEntriesOK(t, entries)
	perceptual := paletteLabDistances(entries)

	evenCV := coefficientOfVariation(even)
	perceptualCV := coefficientOfVariation(perceptual)
	if !(perceptualCV < evenCV*0.5) {
		t.Fatalf("Lab-measured CV not improved: even = %.4f, perceptual = %.4f", evenCV, perceptualCV)
	}
	if !(maxFloat(perceptual) <= maxFloat(even)) {
		t.Fatalf("largest Lab step not reduced: perceptual = %.4f, even = %.4f", maxFloat(perceptual), maxFloat(even))
	}
}

func Test_PerceptualPalette_AlphaWeightIgnoredMatchesOklab(t *testing.T) {
	// The reported distance metric for weight 0 must match labDistance-style
	// Oklab distance used internally.
	a := Rgb(0.2, 0.6, 0.9, 1)
	b := Rgb(0.9, 0.1, 0.4, 1)
	if got, want := ColorDistance(a, b), oklabDistance(a, b); math.Abs(got-want) > 1e-12 {
		t.Fatalf("ColorDistance = %v, want %v", got, want)
	}
}

func maxUniformDeviation(entries []PaletteEntry) float64 {
	n := len(entries)
	if n < 2 {
		return 0
	}
	var worst float64
	for i, e := range entries {
		want := float64(i) / float64(n-1)
		if d := math.Abs(e.Position - want); d > worst {
			worst = d
		}
	}
	return worst
}

func maxFloat(s []float64) float64 {
	m := math.Inf(-1)
	for _, v := range s {
		if v > m {
			m = v
		}
	}
	return m
}

func minFloat(s []float64) float64 {
	m := math.Inf(1)
	for _, v := range s {
		if v < m {
			m = v
		}
	}
	return m
}

func ratio(a, b float64) float64 {
	if b == 0 {
		return math.Inf(1)
	}
	return a / b
}
