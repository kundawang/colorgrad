package colorgrad

import (
	"math"
	"math/rand"
	"strings"
	"testing"
)

// Rebuild CSS stop text through the existing Css entry point and return the
// gradient plus the number of parsed stops.
func rebuildFromCss(t *testing.T, css string) (Gradient, int) {
	t.Helper()
	gb := NewGradient().Css(css)
	grad, err := gb.Build()
	test(t, err, nil)
	return grad, len(*gb.GetPositions())
}

func countCssStops(css string) int {
	return len(splitByComma(css))
}

func Test_CssStops_ExactLinearRgb(t *testing.T) {
	// Custom finite domain, unevenly spaced stops, including a same-position
	// hard edge and a transparent color.
	grad, err := NewGradient().
		HtmlColors("#ff0000", "#00ff0080", "#0000ff", "#ffff00", "#000000").
		Domain(13, 27.3, 27.3, 60, 90).
		Mode(BlendRgb).
		Interpolation(InterpolationLinear).
		Build()
	test(t, err, nil)

	css, err := grad.CssStops()
	test(t, err, nil)
	test(t, countCssStops(css), 5)

	// A hard edge must be serialized as two stops sharing one position.
	parsed, ok := parseCss(css)
	testTrue(t, ok)
	edgeU := norm(27.3, 13, 90)
	testTrue(t, math.Abs(*parsed[1].pos-edgeU) < 1e-12)
	testTrue(t, math.Abs(*parsed[2].pos-edgeU) < 1e-12)
	test(t, *parsed[1].pos, *parsed[2].pos)
	// Transparent color keeps its alpha.
	testTrue(t, strings.Contains(css, "/ 0.5019607843137255"))

	rebuilt, nStops := rebuildFromCss(t, css)
	test(t, nStops, 5)

	// Fixed sample positions across the whole curve must match the original
	// RGBA up to floating point noise, including the hard-edge one-sided
	// limits and the endpoints.
	min, max := grad.Domain()
	for i := 0; i <= 200; i++ {
		u := float64(i) / 200
		want := grad.At(min + u*(max-min))
		got := rebuilt.Core.At(u)
		if colorDistance(want, got) > 1e-12 {
			t.Errorf("at u=%v, want %v, got %v", u, want, got)
		}
	}

	// Hard-edge limits: compare against the original gradient's one-sided
	// limits, since the value exactly at the double-stop position is
	// implementation-defined.
	test(t, rebuilt.Core.At(edgeU-1.0/cssGridRes).HexString(),
		grad.At(13+(edgeU-1.0/cssGridRes)*77).HexString())
	test(t, rebuilt.Core.At(edgeU+1.0/cssGridRes).HexString(),
		grad.At(13+(edgeU+1.0/cssGridRes)*77).HexString())
	test(t, rebuilt.Core.At(0).HexString(), "#ff0000")
	test(t, rebuilt.Core.At(1).HexString(), "#000000")
}

func Test_CssStops_DefaultDomain(t *testing.T) {
	grad, err := NewGradient().
		HtmlColors("red", "white", "blue").
		Build()
	test(t, err, nil)

	css, err := grad.CssStops()
	test(t, err, nil)
	test(t, css, "rgb(100% 0% 0%) 0%, rgb(100% 100% 100%) 50%, rgb(0% 0% 100%) 100%")

	rebuilt, _ := rebuildFromCss(t, css)
	for i := 0; i <= 100; i++ {
		u := float64(i) / 100
		if d := colorDistance(grad.At(u), rebuilt.Core.At(u)); d > 1e-12 {
			t.Errorf("at %v, distance %g", u, d)
		}
	}
}

func Test_CssStops_NegativeDomain(t *testing.T) {
	grad, err := NewGradient().
		HtmlColors("#f00", "#0f0", "#00f").
		Domain(-100, 0, 100).
		Build()
	test(t, err, nil)

	css, err := grad.CssStops()
	test(t, err, nil)
	testTrue(t, strings.HasPrefix(css, "rgb(100% 0% 0%) 0%"))
	testTrue(t, strings.HasSuffix(css, "rgb(0% 0% 100%) 100%"))
	testTrue(t, strings.Contains(css, "50%"))

	rebuilt, _ := rebuildFromCss(t, css)
	for _, u := range []float64{0, 0.25, 0.5, 0.75, 1} {
		want := grad.At(-100 + u*200)
		got := rebuilt.Core.At(u)
		if d := colorDistance(want, got); d > 1e-12 {
			t.Errorf("at %v, distance %g", u, d)
		}
	}
}

func Test_CssStops_UnsupportedTypes(t *testing.T) {
	// Linear interpolation in a non-RGB space.
	lab, err := NewGradient().
		HtmlColors("#f00", "#0f0", "#00f").
		Mode(BlendLab).
		Build()
	test(t, err, nil)
	_, err = lab.CssStops()
	testTrue(t, err != nil)

	// Smoothstep in RGB space.
	smooth, err := NewGradient().
		HtmlColors("#f00", "#0f0", "#00f").
		Interpolation(InterpolationSmoothstep).
		Build()
	test(t, err, nil)
	_, err = smooth.CssStops()
	testTrue(t, err != nil)

	// Basis.
	basis, err := NewGradient().
		HtmlColors("#f00", "#0f0", "#00f").
		Interpolation(InterpolationBasis).
		Build()
	test(t, err, nil)
	_, err = basis.CssStops()
	testTrue(t, err != nil)

	// Catmull-Rom.
	cr, err := NewGradient().
		HtmlColors("#f00", "#0f0", "#00f").
		Interpolation(InterpolationCatmullRom).
		Build()
	test(t, err, nil)
	_, err = cr.CssStops()
	testTrue(t, err != nil)

	// Zero gradient.
	_, err = Gradient{Core: zeroGradient{}}.CssStops()
	testTrue(t, err != nil)
}

func Test_CssStops_DegenerateAndNonFiniteDomain(t *testing.T) {
	linear, err := NewGradient().HtmlColors("#f00", "#0f0").Build()
	test(t, err, nil)

	bad := linear
	bad.Min = 1
	bad.Max = 1
	_, err = bad.CssStops()
	testTrue(t, err != nil)

	bad = linear
	bad.Min = math.NaN()
	_, err = bad.CssStops()
	testTrue(t, err != nil)

	bad = linear
	bad.Max = math.Inf(1)
	_, err = bad.CssStops()
	testTrue(t, err != nil)

	// Approx export must reject the same domains.
	bad = linear
	bad.Min = 0
	bad.Max = 0
	_, err = bad.CssStopsApprox(0.01, 256)
	testTrue(t, err != nil)
}

func Test_CssStopsApprox_AllInterpolations(t *testing.T) {
	builds := map[string]Gradient{}

	grad, err := NewGradient().
		HtmlColors("#ff0000", "#00ff00", "#0000ff", "#ffff00", "#ff00ff").
		Mode(BlendOklab).
		Build()
	test(t, err, nil)
	builds["linear-oklab"] = grad

	grad, err = NewGradient().
		HtmlColors("#ff0000", "#00ff00", "#0000ff", "#ffff00").
		Interpolation(InterpolationSmoothstep).
		Build()
	test(t, err, nil)
	builds["smoothstep-rgb"] = grad

	grad, err = NewGradient().
		HtmlColors("#ff0000", "#00ff00", "#0000ff", "#ffff00").
		Interpolation(InterpolationBasis).
		Mode(BlendLab).
		Build()
	test(t, err, nil)
	builds["basis-lab"] = grad

	grad, err = NewGradient().
		HtmlColors("#ff0000", "#00ff00", "#0000ff", "#ffff00").
		Interpolation(InterpolationCatmullRom).
		Mode(BlendOklab).
		Build()
	test(t, err, nil)
	builds["catmull-rom-oklab"] = grad

	for name, g := range builds {
		t.Run(name, func(t *testing.T) {
			tolerance := 0.01
			css, err := g.CssStopsApprox(tolerance, 512)
			test(t, err, nil)
			nStops := countCssStops(css)
			testTrue(t, nStops <= 512)
			testTrue(t, nStops >= 2)

			rebuilt, _ := rebuildFromCss(t, css)
			min, max := g.Domain()
			// Fixed verification grid.
			for i := 0; i <= 500; i++ {
				u := float64(i) / 500
				want := g.At(min + u*(max-min))
				got := rebuilt.Core.At(u)
				if d := colorDistance(want, got); d > tolerance+1e-9 {
					t.Fatalf("at u=%v distance %g exceeds tolerance %g", u, d, tolerance)
				}
			}
		})
	}
}

func Test_CssStopsApprox_ToleranceAffectsStopCount(t *testing.T) {
	grad, err := NewGradient().
		HtmlColors("#f00", "#0f0", "#00f", "#ff0", "#0ff").
		Interpolation(InterpolationCatmullRom).
		Mode(BlendOklab).
		Build()
	test(t, err, nil)

	loose, err := grad.CssStopsApprox(0.1, 1024)
	test(t, err, nil)
	tight, err := grad.CssStopsApprox(0.001, 1024)
	test(t, err, nil)
	testTrue(t, countCssStops(loose) < countCssStops(tight))

	// Determinism: same inputs produce identical text.
	again, err := grad.CssStopsApprox(0.001, 1024)
	test(t, err, nil)
	test(t, again, tight)
}

func Test_CssStopsApprox_BudgetExceeded(t *testing.T) {
	// A hard-edge gradient can never be approximated within a tolerance
	// smaller than half the jump with only 2 stops.
	grad, err := NewGradient().
		HtmlColors("#fff", "#000").
		Build()
	test(t, err, nil)
	sharp := grad.Sharp(4, 0)

	_, err = sharp.CssStopsApprox(0.01, 3)
	testTrue(t, err != nil)
	testTrue(t, strings.Contains(err.Error(), "at most 3 stops"))

	// A generous budget succeeds and preserves the hard edges. The library
	// guarantee holds on the fixed sampling grid; a hard edge located between
	// grid points is snapped to the nearest grid point.
	css, err := sharp.CssStopsApprox(0.01, 1024)
	test(t, err, nil)
	rebuilt, _ := rebuildFromCss(t, css)
	min, max := sharp.Domain()
	for i := 0; i <= cssGridRes; i++ {
		u := float64(i) / cssGridRes
		want := sharp.At(min + u*(max-min))
		got := rebuilt.Core.At(u)
		if d := colorDistance(want, got); d > 0.01+1e-9 {
			t.Fatalf("at u=%v distance %g", u, d)
		}
	}
	testTrue(t, countCssStops(css) <= 1024)
}

func Test_CssStopsApprox_InvalidArguments(t *testing.T) {
	grad, err := NewGradient().HtmlColors("#f00", "#0f0").Build()
	test(t, err, nil)

	for _, tol := range []float64{-0.01, math.NaN(), math.Inf(1), math.Inf(-1)} {
		_, err = grad.CssStopsApprox(tol, 256)
		testTrue(t, err != nil)
	}

	_, err = grad.CssStopsApprox(0.01, 0)
	testTrue(t, err != nil)
	_, err = grad.CssStopsApprox(0.01, 1)
	testTrue(t, err != nil)
}

func Test_CssStopsApprox_TransparentAndLooseTolerance(t *testing.T) {
	grad, err := NewGradient().
		Colors(Rgb(0, 0, 0, 0), Rgb(1, 0, 0, 1), Rgb(0, 1, 0, 0.25)).
		Interpolation(InterpolationSmoothstep).
		Build()
	test(t, err, nil)

	// A very loose tolerance should fit with just the two endpoints.
	css, err := grad.CssStopsApprox(1, 8)
	test(t, err, nil)
	test(t, countCssStops(css), 2)
	testTrue(t, strings.Contains(css, "/ 0"))

	rebuilt, _ := rebuildFromCss(t, css)
	test(t, rebuilt.Core.At(0).A, 0.0)

	// Tight tolerance reproduces the alpha curve.
	css, err = grad.CssStopsApprox(0.005, 512)
	test(t, err, nil)
	rebuilt, _ = rebuildFromCss(t, css)
	min, max := grad.Domain()
	for i := 0; i <= 300; i++ {
		u := float64(i) / 300
		want := grad.At(min + u*(max-min))
		got := rebuilt.Core.At(u)
		if d := colorDistance(want, got); d > 0.005+1e-9 {
			t.Fatalf("at u=%v distance %g", u, d)
		}
	}
}

func Test_CssStopsApprox_LinearRgbRounding(t *testing.T) {
	// Linear-light interpolation produces out-of-sRGB curves; the approx
	// path handles it and respects the tolerance on the RGBA output.
	grad, err := NewGradient().
		HtmlColors("#123456", "#fedcba", "#abcdef").
		Mode(BlendLinearRgb).
		Build()
	test(t, err, nil)

	css, err := grad.CssStopsApprox(0.002, 256)
	test(t, err, nil)
	rebuilt, _ := rebuildFromCss(t, css)
	min, max := grad.Domain()
	for i := 0; i <= 400; i++ {
		u := float64(i) / 400
		want := grad.At(min + u*(max-min))
		got := rebuilt.Core.At(u)
		if d := colorDistance(want, got); d > 0.002+1e-9 {
			t.Fatalf("at u=%v distance %g", u, d)
		}
	}
}

func Test_CssStopsApprox_NonFiniteColor(t *testing.T) {
	grad, err := NewGradient().HtmlColors("#f00", "#0f0").Build()
	test(t, err, nil)

	bad := grad
	bad.Core = nonFiniteGradient{}
	_, err = bad.CssStopsApprox(0.01, 256)
	testTrue(t, err != nil)
	testTrue(t, strings.Contains(err.Error(), "non-finite"))
}

func Test_CssStops_NonFiniteStopsAndColors(t *testing.T) {
	grad, err := NewGradient().HtmlColors("#f00", "#0f0").Build()
	test(t, err, nil)

	// Non-finite stop position stored in the underlying gradient.
	lg := grad.Core.(linearGradient)
	badPos := grad
	lg.positions = append([]float64(nil), lg.positions...)
	lg.positions[0] = math.NaN()
	badPos.Core = lg
	_, err = badPos.CssStops()
	testTrue(t, err != nil)
	testTrue(t, strings.Contains(err.Error(), "non-finite stop position"))

	// Non-finite color stored in the underlying gradient.
	lg2 := grad.Core.(linearGradient)
	badCol := grad
	lg2.colors = append([][4]float64(nil), lg2.colors...)
	lg2.colors[0] = [4]float64{math.Inf(1), 0, 0, 1}
	badCol.Core = lg2
	_, err = badCol.CssStops()
	testTrue(t, err != nil)
	testTrue(t, strings.Contains(err.Error(), "non-finite color value"))
}

func Test_CssStopsApprox_SmoothCurveBudgetExceeded(t *testing.T) {
	// A high-frequency smooth curve cannot fit a tiny stop budget.
	grad, err := NewGradient().
		HtmlColors("#f00", "#0f0", "#00f", "#ff0", "#0ff", "#f0f").
		Interpolation(InterpolationCatmullRom).
		Mode(BlendOklab).
		Build()
	test(t, err, nil)

	_, err = grad.CssStopsApprox(0.0001, 3)
	testTrue(t, err != nil)
	testTrue(t, strings.Contains(err.Error(), "at most 3 stops"))
}

func Test_CssStopsApprox_DomainRoundTrip(t *testing.T) {
	// A non-unit finite domain is normalized: rebuilt gradient lives on
	// [0, 1] and reproduces the original at normalized positions.
	grad, err := NewGradient().
		HtmlColors("#111", "#eee", "#336699").
		Domain(5, 25, 75).
		Interpolation(InterpolationSmoothstep).
		Build()
	test(t, err, nil)

	css, err := grad.CssStopsApprox(0.005, 256)
	test(t, err, nil)
	rebuilt, nStops := rebuildFromCss(t, css)
	testTrue(t, nStops <= 256)
	dmin, dmax := rebuilt.Domain()
	test(t, dmin, 0.0)
	test(t, dmax, 1.0)

	min, max := grad.Domain()
	for i := 0; i <= 300; i++ {
		u := float64(i) / 300
		if d := colorDistance(grad.At(min+u*(max-min)), rebuilt.Core.At(u)); d > 0.005+1e-9 {
			t.Fatalf("at u=%g distance %g", u, d)
		}
	}
}

type nonFiniteGradient struct{}

func (nonFiniteGradient) At(float64) Color {
	return Color{R: math.NaN()}
}

// Fixed-seed stress test covering every blend mode and interpolation,
// transparent colors, hard edges, zero tolerance and large domains. It
// verifies the round trip on the library's fixed sampling grid and checks
// that identical inputs always produce identical text.
func Test_CssStopsApprox_Stress(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	modes := []BlendMode{BlendRgb, BlendLinearRgb, BlendLab, BlendOklab}
	interps := []Interpolation{InterpolationLinear, InterpolationSmoothstep, InterpolationBasis, InterpolationCatmullRom}

	for iter := 0; iter < 200; iter++ {
		n := 2 + rng.Intn(6)
		cols := make([]Color, n)
		for i := range cols {
			cols[i] = Rgb(rng.Float64(), rng.Float64(), rng.Float64(), rng.Float64())
		}
		g, err := NewGradient().
			Colors(cols...).
			Mode(modes[rng.Intn(len(modes))]).
			Interpolation(interps[rng.Intn(len(interps))]).
			Build()
		test(t, err, nil)

		tol := 0.001 + rng.Float64()*0.05
		css, err := g.CssStopsApprox(tol, 4096)
		test(t, err, nil)
		rebuilt, berr := NewGradient().Css(css).Build()
		test(t, berr, nil)
		for i := 0; i <= cssGridRes; i++ {
			u := float64(i) / cssGridRes
			if d := colorDistance(g.At(u), rebuilt.Core.At(u)); d > tol+1e-9 {
				t.Fatalf("iter %d at u=%g distance %g exceeds tolerance %g", iter, u, d, tol)
			}
		}

		again, err := g.CssStopsApprox(tol, 4096)
		test(t, err, nil)
		test(t, again, css)
	}

	// Sharp gradients place hard edges at many positions, including ones
	// that do not line up with grid points.
	for seg := uint(2); seg <= 12; seg++ {
		base, err := NewGradient().Colors(Rgb(0, 0, 0, 0), Rgb(1, 1, 1, 1)).Build()
		test(t, err, nil)
		sg := base.Sharp(seg, rng.Float64()*0.5)
		css, err := sg.CssStopsApprox(0.02, 4096)
		test(t, err, nil)
		rebuilt, berr := NewGradient().Css(css).Build()
		test(t, berr, nil)
		for i := 0; i <= cssGridRes; i++ {
			u := float64(i) / cssGridRes
			if d := colorDistance(sg.At(u), rebuilt.Core.At(u)); d > 0.02+1e-9 {
				t.Fatalf("sharp seg %d at u=%g distance %g", seg, u, d)
			}
		}
	}

	// A constant gradient fits in exactly two stops at any tolerance.
	constg, err := NewGradient().Colors(Rgb(0.3, 0.4, 0.5, 0.6), Rgb(0.3, 0.4, 0.5, 0.6)).Build()
	test(t, err, nil)
	css, err := constg.CssStopsApprox(0.001, 2)
	test(t, err, nil)
	test(t, countCssStops(css), 2)

	// Zero tolerance is valid and exact for a piecewise-linear RGB gradient.
	lin, err := NewGradient().HtmlColors("#f00", "#0f0", "#00f").Build()
	test(t, err, nil)
	css, err = lin.CssStopsApprox(0, 4096)
	test(t, err, nil)
	rebuilt, berr := NewGradient().Css(css).Build()
	test(t, berr, nil)
	for i := 0; i <= cssGridRes; i++ {
		u := float64(i) / cssGridRes
		if d := colorDistance(lin.At(u), rebuilt.Core.At(u)); d > 1e-9 {
			t.Fatalf("zero tolerance at u=%g distance %g", u, d)
		}
	}

	// A large finite domain is mapped to percentages without overflow.
	big, err := NewGradient().HtmlColors("#f00", "#00f").Domain(-1e9, 1e9).Build()
	test(t, err, nil)
	css, err = big.CssStops()
	test(t, err, nil)
	rebuilt, berr = NewGradient().Css(css).Build()
	test(t, berr, nil)
	testTrue(t, math.Abs(rebuilt.Core.At(0).R-1) < 1e-12)
	testTrue(t, math.Abs(rebuilt.Core.At(1).B-1) < 1e-12)
}
