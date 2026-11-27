package colorgrad

import (
	"math"
	"strings"
	"testing"
)

// --- Test helpers ---------------------------------------------------------

func assertNear(t *testing.T, got, want, tol float64, msg string) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s: got %.10f, want %.10f (tol %g)", msg, got, want, tol)
	}
}

func assertVec4Near(t *testing.T, got, want [4]float64, tol float64, msg string) {
	t.Helper()
	for i := 0; i < 4; i++ {
		if math.Abs(got[i]-want[i]) > tol {
			t.Errorf("%s channel %d: got %.10f, want %.10f (tol %g)", msg, i, got[i], want[i], tol)
		}
	}
}

// premultLinear converts an sRGB color back to premultiplied linear-light
// channels, the space in which the interval average is defined.
func premultLinear(c Color) [4]float64 {
	a := c.A
	return [4]float64{toLinear(c.R) * a, toLinear(c.G) * a, toLinear(c.B) * a, a}
}

// refAverage is an independent, high-resolution midpoint-rule reference
// integrator using the same premultiplied linear-light pipeline.
func refAverage(g Gradient, t0, t1 float64, samples int) Color {
	if t0 == t1 {
		return g.At(t0).Clamp()
	}
	var sum [4]float64
	width := t1 - t0
	for i := 0; i < samples; i++ {
		t := t0 + (float64(i)+0.5)*width/float64(samples)
		p := g.premultipliedLinearSample(t)
		for k := range sum {
			sum[k] += p[k]
		}
	}
	for k := range sum {
		sum[k] /= float64(samples)
	}
	if sum[3] <= 0 {
		return Color{}
	}
	return LinearRgb(sum[0]/sum[3], sum[1]/sum[3], sum[2]/sum[3], sum[3]).Clamp()
}

func assertValidFinite(t *testing.T, c Color, msg string) {
	t.Helper()
	vals := [4]float64{c.R, c.G, c.B, c.A}
	for i, v := range vals {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 1 {
			t.Errorf("%s: invalid channel %d = %v", msg, i, v)
		}
	}
}

// --- Fixed samples --------------------------------------------------------

// Two equal-width solid colors in a hard-edge gradient: the integral is
// exact, so analytic expectations can be asserted tightly.
func Test_AverageColor_TwoSolidBands(t *testing.T) {
	linR := toLinear(1.0)
	linG := toLinear(1.0) // green: R=0, G=1, B=0 -> only G channel

	grad, err := NewGradient().HtmlColors("#f00", "#0f0").Build()
	testTrue(t, err == nil)
	sharp := grad.Sharp(2, 0)

	c, err := sharp.AverageColor(0, 1)
	testTrue(t, err == nil)
	assertValidFinite(t, c, "two solid bands")

	// Half red, half green, both opaque: premultiplied linear average.
	want := [4]float64{linR / 2, linG / 2, 0, 1}
	assertVec4Near(t, premultLinear(c), want, 1e-9, "two solid bands")

	// Same interval queried with a non-default domain mapping.
	gradD, err := NewGradient().HtmlColors("#f00", "#0f0").Domain(10, 20).Build()
	testTrue(t, err == nil)
	sharpD := gradD.Sharp(2, 0)
	cD, err := sharpD.AverageColor(10, 20)
	testTrue(t, err == nil)
	assertVec4Near(t, premultLinear(cD), want, 1e-9, "two solid bands (domain 10..20)")
}

// Two equal-width bands where the second one is semi-transparent. This is
// the fixed sample that proves transparent colors do not leak dark edges:
// the transparent region contributes zero premultiplied RGB.
func Test_AverageColor_SemiTransparentHardEdge(t *testing.T) {
	linR := toLinear(1.0)
	linB := toLinear(1.0) // blue: B=1
	grad, err := NewGradient().
		Colors(Rgb(1, 0, 0, 1), Rgb(0, 0, 1, 0.5)).
		Build()
	testTrue(t, err == nil)
	sharp := grad.Sharp(2, 0)

	c, err := sharp.AverageColor(0, 1)
	testTrue(t, err == nil)
	assertValidFinite(t, c, "semi-transparent hard edge")

	// alpha: (1 + 0.5) / 2
	assertNear(t, c.A, 0.75, 1e-9, "average alpha")

	// Premultiplied linear integrals:
	// red band contributes 0.5*linR; transparent-blue band contributes
	// 0.5 (width) * 0.5 (alpha) * linB and nothing where alpha is zero.
	wantPremult := [4]float64{0.5 * linR, 0, 0.25 * linB, 0.75}
	gotPremult := premultLinear(c)
	assertVec4Near(t, gotPremult, wantPremult, 1e-9, "premultiplied average")

	// A straight (non-premultiplied) average would give the blue channel an
	// effective premultiplied value of 0.5*linB, i.e. twice the correct one,
	// showing why premultiplication is required.
	if gotPremult[2] > 0.30*linB {
		t.Errorf("transparent region leaked blue: %.10f", gotPremult[2])
	}

	// Unpremultiplied color of the visible part.
	assertNear(t, toLinear(c.R), linR*0.5/0.75, 1e-9, "unpremult red")
	assertNear(t, toLinear(c.B), linB*0.25/0.75, 1e-9, "unpremult blue")
}

// A 1e-6-wide solid green band between red and blue: a single center sample
// would miss it, but the interval integral must weight its coverage exactly.
func Test_AverageColor_NarrowBand(t *testing.T) {
	linR := toLinear(1.0)
	linG := toLinear(1.0)
	linB := toLinear(1.0)

	grad, err := NewGradient().
		Colors(
			Rgb(1, 0, 0, 1), // red
			Rgb(1, 0, 0, 1), // red
			Rgb(0, 1, 0, 1), // green band start
			Rgb(0, 1, 0, 1), // green band end
			Rgb(0, 0, 1, 1), // blue
			Rgb(0, 0, 1, 1), // blue
		).
		Domain(0, 0.4999995, 0.4999995, 0.5000005, 0.5000005, 1).
		Build()
	testTrue(t, err == nil)

	// Interval of width 2e-6 around the 1e-6-wide band: the solid green band
	// covers half of it, red and blue a quarter each.
	c, err := grad.AverageColor(0.499999, 0.500001)
	testTrue(t, err == nil)
	assertValidFinite(t, c, "narrow band")

	want := [4]float64{0.25 * linR, 0.5 * linG, 0.25 * linB, 1}
	assertVec4Near(t, premultLinear(c), want, 1e-6, "narrow band coverage")

	// The whole-domain average agrees with a dense reference.
	ref := refAverage(grad, 0, 1, 200001)
	got, err := grad.AverageColor(0, 1)
	testTrue(t, err == nil)
	assertVec4Near(t, premultLinear(got), premultLinear(ref), 1e-4, "narrow band full domain")
}

// Hard-edge thirds with an interval straddling one boundary: exact piece
// weights, no band lost at the discontinuity.
func Test_AverageColor_SharpBoundaryWeights(t *testing.T) {
	linR := toLinear(1.0)
	linG := toLinear(1.0)

	grad, err := NewGradient().HtmlColors("#f00", "#0f0", "#00f").Build()
	testTrue(t, err == nil)
	sharp := grad.Sharp(3, 0)

	// [0.3, 1/3): red width 1/30, [1/3, 0.36]: green width 2/75.
	// Fractions: red = 5/9, green = 4/9.
	c, err := sharp.AverageColor(0.3, 0.36)
	testTrue(t, err == nil)
	want := [4]float64{5.0 / 9.0 * linR, 4.0 / 9.0 * linG, 0, 1}
	assertVec4Near(t, premultLinear(c), want, 1e-9, "sharp boundary weights")
}

// --- All gradient kinds against the dense reference integrator ------------

func Test_AverageColor_AllInterpolations(t *testing.T) {
	htmls := []string{"#f00", "#ff880088", "#0f0", "#00f", "#8000ff"}
	interpolations := []Interpolation{
		InterpolationLinear,
		InterpolationSmoothstep,
		InterpolationBasis,
		InterpolationCatmullRom,
	}
	modes := []BlendMode{BlendRgb, BlendLinearRgb, BlendLab, BlendOklab}

	for _, interp := range interpolations {
		for _, mode := range modes {
			grad, err := NewGradient().
				HtmlColors(htmls...).
				Interpolation(interp).
				Mode(mode).
				Build()
			testTrue(t, err == nil)

			got, err := grad.AverageColor(0.13, 0.87)
			testTrue(t, err == nil)
			assertValidFinite(t, got, interp.String()+"/"+mode.String())

			ref := refAverage(grad, 0.13, 0.87, 20001)
			assertVec4Near(t, premultLinear(got), premultLinear(ref), 1e-3,
				interp.String()+"/"+mode.String())

			// Determinism.
			again, err := grad.AverageColor(0.13, 0.87)
			testTrue(t, err == nil)
			test(t, got, again)
		}
	}
}

func Test_AverageColor_Presets(t *testing.T) {
	// Preset cores do not expose stop positions, so integration runs on a
	// single piece; the functions are smooth and must still match the
	// dense reference.
	for name, grad := range map[string]Gradient{
		"Sinebow":   Sinebow(),
		"Turbo":     Turbo(),
		"Cividis":   Cividis(),
		"Cubehelix": CubehelixDefault(),
		"Warm":      Warm(),
		"Cool":      Cool(),
	} {
		got, err := grad.AverageColor(0, 1)
		testTrue(t, err == nil)
		assertValidFinite(t, got, name)
		ref := refAverage(grad, 0, 1, 20001)
		assertVec4Near(t, premultLinear(got), premultLinear(ref), 1e-3, name)
	}
}

func Test_AverageColor_GGR(t *testing.T) {
	black := Rgb(0, 0, 0, 1)
	// Two segments with a hard step at t=0.5 (blending type 5 = step).
	ggr := "GIMP Gradient\nName: G\n2\n0 0.25 0.5 1 0 0 1 0 0 1 1 5 0 0 0\n0.5 0.75 1 0 0 0 1 1 1 1 1 5 0 0 0"
	grad, _, err := ParseGgr(strings.NewReader(ggr), black, black)
	testTrue(t, err == nil)

	got, err := grad.AverageColor(0, 1)
	testTrue(t, err == nil)
	assertValidFinite(t, got, "ggr")
	ref := refAverage(grad, 0, 1, 20001)
	assertVec4Near(t, premultLinear(got), premultLinear(ref), 1e-3, "ggr")

	// Zero-length query on a GGR gradient.
	c, err := grad.AverageColor(0.3, 0.3)
	testTrue(t, err == nil)
	test(t, c, grad.At(0.3).Clamp())
}

func Test_AverageColor_SharpSmoothness(t *testing.T) {
	grad, err := NewGradient().
		HtmlColors("#f00", "#0f0", "#00f", "#ff0").
		Build()
	testTrue(t, err == nil)

	for _, smoothness := range []float64{0, 0.15, 0.5} {
		sharp := grad.Sharp(4, smoothness)
		got, err := sharp.AverageColor(0.07, 0.93)
		testTrue(t, err == nil)
		assertValidFinite(t, got, "sharp smoothness")
		ref := refAverage(sharp, 0.07, 0.93, 40001)
		assertVec4Near(t, premultLinear(got), premultLinear(ref), 1e-3, "sharp smoothness")
	}
}

// --- Degenerate and boundary inputs ---------------------------------------

func Test_AverageColor_ZeroLength(t *testing.T) {
	grad, err := NewGradient().HtmlColors("#f00", "#0f0", "#00f").Build()
	testTrue(t, err == nil)

	for _, t0 := range []float64{0, 0.3, 0.5, 1} {
		c, err := grad.AverageColor(t0, t0)
		testTrue(t, err == nil)
		test(t, c, grad.At(t0).Clamp())
	}

	// Empty after domain clamping still yields the clamped endpoint color.
	gradD, err := NewGradient().HtmlColors("#f00", "#0f0").Domain(10, 20).Build()
	testTrue(t, err == nil)
	c, err := gradD.AverageColor(0, 5)
	testTrue(t, err == nil)
	test(t, c, gradD.At(10).Clamp())

	c, err = gradD.AverageColor(25, 30)
	testTrue(t, err == nil)
	test(t, c, gradD.At(20).Clamp())
}

func Test_AverageColor_NonFinite(t *testing.T) {
	grad, err := NewGradient().Build()
	testTrue(t, err == nil)

	bad := []float64{math.NaN(), math.Inf(1), math.Inf(-1)}
	for _, b := range bad {
		if c, err := grad.AverageColor(b, 1); err == nil {
			t.Errorf("expected error for t0=%v", b)
		} else if c != (Color{}) {
			t.Errorf("expected zero color for t0=%v, got %v", b, c)
		}
		if c, err := grad.AverageColor(0, b); err == nil {
			t.Errorf("expected error for t1=%v", b)
		} else if c != (Color{}) {
			t.Errorf("expected zero color for t1=%v, got %v", b, c)
		}
	}
}

func Test_AverageColor_OutsideDomain(t *testing.T) {
	grad, err := NewGradient().HtmlColors("#f00", "#0f0").Build()
	testTrue(t, err == nil)

	inside, err := grad.AverageColor(0, 1)
	testTrue(t, err == nil)
	outside, err := grad.AverageColor(-1, 2)
	testTrue(t, err == nil)
	test(t, inside, outside)

	// Constant extension outside the domain equals the endpoint color.
	left, err := grad.AverageColor(-2, -1)
	testTrue(t, err == nil)
	test(t, left, grad.At(0).Clamp())
	right, err := grad.AverageColor(2, 3)
	testTrue(t, err == nil)
	test(t, right, grad.At(1).Clamp())
}

func Test_AverageColor_ReversedInterval(t *testing.T) {
	grad, err := NewGradient().HtmlColors("#f00", "#0f0", "#00f").Build()
	testTrue(t, err == nil)

	a, err := grad.AverageColor(0.2, 0.8)
	testTrue(t, err == nil)
	b, err := grad.AverageColor(0.8, 0.2)
	testTrue(t, err == nil)
	test(t, a, b)
}

func Test_AverageColor_FullyTransparent(t *testing.T) {
	grad, err := NewGradient().
		Colors(Rgb(1, 0, 0, 0), Rgb(0, 0, 1, 0)).
		Build()
	testTrue(t, err == nil)

	c, err := grad.AverageColor(0, 1)
	testTrue(t, err == nil)
	test(t, c, Color{})

	// Zero gradient (failed build) behaves the same way.
	zg := Gradient{Core: zeroGradient{}, Min: 0, Max: 1}
	c, err = zg.AverageColor(0, 1)
	testTrue(t, err == nil)
	test(t, c, Color{})
}

func Test_AverageColor_PrecisionOptions(t *testing.T) {
	grad, err := NewGradient().
		HtmlColors("#f00", "#ff880088", "#0f0", "#00f").
		Interpolation(InterpolationCatmullRom).
		Build()
	testTrue(t, err == nil)

	// Invalid options fall back to defaults instead of panicking.
	for _, eps := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		c, err := grad.AverageColor(0.1, 0.9, AverageEpsilon(eps))
		testTrue(t, err == nil)
		assertValidFinite(t, c, "bad epsilon")
	}
	for _, depth := range []int{0, -5, 999} {
		c, err := grad.AverageColor(0.1, 0.9, AverageMaxDepth(depth))
		testTrue(t, err == nil)
		assertValidFinite(t, c, "bad depth")
	}

	// Tight precision tracks the dense reference more closely than loose.
	ref := premultLinear(refAverage(grad, 0.1, 0.9, 40001))
	loose, err := grad.AverageColor(0.1, 0.9, AverageEpsilon(1e-2), AverageMaxDepth(1))
	testTrue(t, err == nil)
	tight, err := grad.AverageColor(0.1, 0.9, AverageEpsilon(1e-10), AverageMaxDepth(20))
	testTrue(t, err == nil)

	diffLoose := 0.0
	diffTight := 0.0
	for k := 0; k < 4; k++ {
		diffLoose += math.Abs(premultLinear(loose)[k] - ref[k])
		diffTight += math.Abs(premultLinear(tight)[k] - ref[k])
	}
	if diffTight > diffLoose {
		t.Errorf("tight precision (%.2e) not better than loose (%.2e)", diffTight, diffLoose)
	}
}
