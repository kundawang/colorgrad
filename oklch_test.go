package colorgrad

import (
	"math"
	"testing"
)

const degToRad = math.Pi / 180

// hueDistance returns the shortest absolute distance between two hue angles
// in degrees, so that 359 and 1 are 2 degrees apart, not 358.
func hueDistance(a, b float64) float64 {
	d := math.Abs(normalizeAngle(a) - normalizeAngle(b))
	if d > 180 {
		d = 360 - d
	}
	return d
}

func oklchComponents(c Color) (l, chroma, hue float64) {
	a := col2oklch(c)
	return a[0], a[1], a[2]
}

func mustOklch(l, c, h, a float64) Color {
	return Oklch(l, c, h*degToRad, a)
}

func Test_BlendModeOklchString(t *testing.T) {
	test(t, BlendOklch.String(), "BlendOklch")
	testTrue(t, BlendOklch != BlendOklab)
}

// A 350° -> 10° gradient must take the 20° arc through 0°/360°, never the
// 340° arc through the opposite side of the hue circle.
func Test_OklchShortHueArc(t *testing.T) {
	interpolations := []Interpolation{
		InterpolationLinear,
		InterpolationSmoothstep,
		InterpolationCatmullRom,
		InterpolationBasis,
	}

	for _, ip := range interpolations {
		grad, err := NewGradient().
			Colors(
				mustOklch(0.65, 0.2, 350, 1),
				mustOklch(0.65, 0.2, 10, 1),
			).
			Mode(BlendOklch).
			Interpolation(ip).
			Build()
		test(t, err, nil)

		// Domain endpoints are the exact source colors.
		test(t, grad.At(0).HexString(), "#e24ea0")
		test(t, grad.At(1).HexString(), "#ee4a73")

		// Every sample must stay on the short arc, with lightness and chroma
		// close to the constant values of the stops.
		for i := 0; i <= 200; i++ {
			p := float64(i) / 200
			col := grad.At(p)
			l, c, h := oklchComponents(col)

			onArc := hueDistance(h, 350) <= 20.001 || hueDistance(h, 10) <= 20.001
			testTrue(t, onArc)
			testTrue(t, math.Abs(l-0.65) < 0.01)
			testTrue(t, math.Abs(c-0.2) < 0.01)
			testTrue(t, math.Abs(col.A-1) < 1e-12)
		}

		// The midpoint hue is near 0°, i.e. the short way around.
		_, _, mid := oklchComponents(grad.At(0.5))
		testTrue(t, hueDistance(mid, 0) < 1.0)
	}
}

// Hues close to 0° but with a larger gap still go the short way.
func Test_OklchWrapAroundBothDirections(t *testing.T) {
	cases := []struct {
		from, to float64
	}{
		{350, 10},
		{10, 350},
		{355, 5},
		{5, 355},
		{340, 20},
		{20, 340},
	}

	for _, c := range cases {
		grad, err := NewGradient().
			Colors(
				mustOklch(0.65, 0.2, c.from, 1),
				mustOklch(0.65, 0.2, c.to, 1),
			).
			Mode(BlendOklch).
			Build()
		test(t, err, nil)

		_, _, mid := oklchComponents(grad.At(0.5))
		// Midpoint hue is the halfway point along the chosen short arc.
		d := c.to - c.from
		if d > 180 {
			d -= 360
		} else if d < -180 {
			d += 360
		}
		shortMid := c.from + d/2
		testTrue(t, hueDistance(mid, shortMid) < 0.5)
	}
}

// Achromatic stops have no meaningful hue; the numerical hue noise they carry
// must not introduce a hue jump. Both a gray and a white endpoint should
// continue the hue of their chromatic neighbor.
func Test_OklchAchromaticStops(t *testing.T) {
	grad, err := NewGradient().
		HtmlColors("#808080", "#ff0080", "#ffffff").
		Mode(BlendOklch).
		Build()
	test(t, err, nil)

	// Endpoints are preserved exactly.
	test(t, grad.At(0).HexString(), "#808080")
	test(t, grad.At(1).HexString(), "#ffffff")

	// The chromatic stop is hit exactly.
	test(t, grad.At(0.5).HexString(), "#ff0080")

	// Between the gray/white ends and the magenta stop, hue follows magenta
	// instead of jumping to the gray's numerical hue (~274°).
	_, _, magentaHue := oklchComponents(grad.At(0.5))
	for _, p := range []float64{0.1, 0.25, 0.4, 0.6, 0.75, 0.9} {
		_, _, h := oklchComponents(grad.At(p))
		testTrue(t, hueDistance(h, magentaHue) < 60)
	}

	// All four interpolations remain valid.
	for _, ip := range []Interpolation{
		InterpolationLinear,
		InterpolationSmoothstep,
		InterpolationCatmullRom,
		InterpolationBasis,
	} {
		g, err := NewGradient().
			HtmlColors("#808080", "#ff0080", "#ffffff").
			Mode(BlendOklch).
			Interpolation(ip).
			Build()
		test(t, err, nil)
		test(t, g.At(0).HexString(), "#808080")
		test(t, g.At(1).HexString(), "#ffffff")
		assertValidColors(t, g, 0, 1, 500)
	}

	// An all-achromatic gradient stays a plain gray ramp.
	gray, err := NewGradient().
		HtmlColors("#000", "#888", "#fff").
		Mode(BlendOklch).
		Build()
	test(t, err, nil)
	test(t, gray.At(0).HexString(), "#000000")
	test(t, gray.At(0.5).HexString(), "#888888")
	test(t, gray.At(1).HexString(), "#ffffff")
}

// 8-bit near-grays pick up a tiny chroma with a noisy hue. That noise must not
// be mistaken for a real hue, otherwise a neutral stop would drag the ramp
// toward an unrelated color.
func Test_OklchNearGrayNoise(t *testing.T) {
	grad, err := NewGradient().
		HtmlColors("#f0f0f0", "#ff0080").
		Mode(BlendOklch).
		Build()
	test(t, err, nil)

	test(t, grad.At(0).HexString(), "#f0f0f0")
	test(t, grad.At(1).HexString(), "#ff0080")

	_, _, magentaHue := oklchComponents(grad.At(1))
	for i := 10; i <= 100; i++ {
		p := float64(i) / 100
		_, _, h := oklchComponents(grad.At(p))
		testTrue(t, hueDistance(h, magentaHue) < 5)
	}
}

// Multiple stops whose hues zig-zag in reverse must use the short arc for
// each segment, producing a V shape through 0° rather than a full loop.
func Test_OklchMultiStopReverseHue(t *testing.T) {
	grad, err := NewGradient().
		Colors(
			mustOklch(0.65, 0.2, 10, 1),
			mustOklch(0.65, 0.2, 350, 1),
			mustOklch(0.65, 0.2, 10, 1),
		).
		Mode(BlendOklch).
		Interpolation(InterpolationLinear).
		Build()
	test(t, err, nil)

	samples := []struct {
		p   float64
		hue float64
	}{
		{0.000, 10},
		{0.125, 5},
		{0.250, 0},
		{0.375, 355},
		{0.500, 350},
		{0.625, 355},
		{0.750, 0},
		{0.875, 5},
		{1.000, 10},
	}
	for _, s := range samples {
		_, _, h := oklchComponents(grad.At(s.p))
		testTrue(t, hueDistance(h, s.hue) < 1.0)
	}

	// No sample may escape the two-segment short arcs.
	for i := 0; i <= 500; i++ {
		_, _, h := oklchComponents(grad.At(float64(i) / 500))
		testTrue(t, hueDistance(h, 350) <= 20.001 || hueDistance(h, 10) <= 20.001)
	}

	// A hue sequence that keeps going one way (10 -> 350 -> 300) must also be
	// continuous: 10 -> 350 goes backwards 20°, 350 -> 300 goes backwards 50°.
	mixed, err := NewGradient().
		Colors(
			mustOklch(0.65, 0.2, 10, 1),
			mustOklch(0.65, 0.2, 350, 1),
			mustOklch(0.65, 0.2, 300, 1),
		).
		Mode(BlendOklch).
		Build()
	test(t, err, nil)
	_, _, h1 := oklchComponents(mixed.At(0.25))
	_, _, h2 := oklchComponents(mixed.At(0.75))
	testTrue(t, hueDistance(h1, 0) < 1.0)
	testTrue(t, hueDistance(h2, 325) < 1.0)
}

// Lightness, chroma and alpha are interpolated independently and stay valid.
func Test_OklchComponentsAndAlpha(t *testing.T) {
	grad, err := NewGradient().
		Colors(
			mustOklch(0.4, 0.1, 350, 0),
			mustOklch(0.6, 0.2, 10, 1),
		).
		Mode(BlendOklch).
		Build()
	test(t, err, nil)

	for i := 0; i <= 100; i++ {
		p := float64(i) / 100
		col := grad.At(p)
		l, c, _ := oklchComponents(col)
		testTrue(t, math.Abs(l-(0.4+0.2*p)) < 0.005)
		testTrue(t, math.Abs(c-(0.1+0.1*p)) < 0.005)
		testTrue(t, math.Abs(col.A-p) < 0.005)
	}
}

// Domain endpoints, clamping outside the domain, and hard edges at coincident
// stop positions must all be preserved.
func Test_OklchDomainAndHardEdge(t *testing.T) {
	grad, err := NewGradient().
		Colors(
			mustOklch(0.65, 0.2, 350, 1),
			mustOklch(0.65, 0.2, 10, 1),
		).
		Domain(10, 50).
		Mode(BlendOklch).
		Build()
	test(t, err, nil)

	lo, hi := grad.Domain()
	test(t, lo, 10.0)
	test(t, hi, 50.0)
	test(t, grad.At(10).HexString(), "#e24ea0")
	test(t, grad.At(50).HexString(), "#ee4a73")
	test(t, grad.At(9).HexString(), grad.At(10).HexString())
	test(t, grad.At(51).HexString(), grad.At(50).HexString())

	// Coincident positions 0.5, 0.5 create a hard edge: the stop on each side
	// is exact and the color jumps between them.
	hard, err := NewGradient().
		Colors(
			mustOklch(0.65, 0.2, 350, 1),
			mustOklch(0.65, 0.2, 10, 1),
			mustOklch(0.65, 0.2, 200, 1),
			mustOklch(0.65, 0.2, 350, 1),
		).
		Domain(0, 0.5, 0.5, 1).
		Mode(BlendOklch).
		Build()
	test(t, err, nil)

	test(t, hard.At(0).HexString(), "#e24ea0")
	test(t, hard.At(0.25).HexString(), "#e94b8a")
	test(t, hard.At(0.5).HexString(), "#ee4a73")
	test(t, hard.At(0.75).HexString(), "#6f7dff")
	test(t, hard.At(1).HexString(), "#e24ea0")

	// Just below and just at the edge select different stops.
	test(t, hard.At(0.4999).HexString(), "#ee4a73")
	testTrue(t, hard.At(0.4999) != hard.At(0.5001))
}

// Every interpolation must produce valid, in-range RGBA across the whole
// domain and outside it, including NaN input handling.
func Test_OklchProducesValidColors(t *testing.T) {
	for _, ip := range []Interpolation{
		InterpolationLinear,
		InterpolationSmoothstep,
		InterpolationCatmullRom,
		InterpolationBasis,
	} {
		grad, err := NewGradient().
			HtmlColors("#ff0000", "#00ff00", "#0000ff").
			Mode(BlendOklch).
			Interpolation(ip).
			Build()
		test(t, err, nil)

		assertValidColors(t, grad, -0.5, 1.5, 1000)
		test(t, grad.At(math.NaN()).HexString(), "#000000")
	}
}

// Building the same gradient twice must give identical colors (no shared
// mutable state, no dependence on evaluation order).
func Test_OklchRepeatableBuild(t *testing.T) {
	build := func() Gradient {
		g, err := NewGradient().
			HtmlColors("#ff0000", "#808080", "#00ff00", "#0000ff", "#ffffff").
			Mode(BlendOklch).
			Interpolation(InterpolationCatmullRom).
			Build()
		test(t, err, nil)
		return g
	}

	g1 := build()
	g2 := build()
	for i := 0; i <= 500; i++ {
		p := float64(i) / 500
		test(t, g1.At(p), g2.At(p))
	}

	// The same builder can also be built twice.
	gb := NewGradient().
		HtmlColors("#ff0000", "#808080", "#00ff00").
		Mode(BlendOklch)
	ga, err := gb.Build()
	test(t, err, nil)
	gb2 := NewGradient().
		HtmlColors("#ff0000", "#808080", "#00ff00").
		Mode(BlendOklch)
	gb_, err := gb2.Build()
	test(t, err, nil)
	for i := 0; i <= 100; i++ {
		p := float64(i) / 100
		test(t, ga.At(p), gb_.At(p))
	}
}

// The existing modes must be untouched: these are the pre-change outputs of
// the shared red/green/blue sample for BlendRgb and BlendOklab.
func Test_OklchDoesNotChangeExistingModes(t *testing.T) {
	rgb, err := NewGradient().
		HtmlColors("#f00", "#0f0", "#00f").
		Mode(BlendRgb).
		Interpolation(InterpolationLinear).
		Build()
	test(t, err, nil)
	testSlice(t, colors2hex(rgb.Colors(5)), []string{
		"#ff0000",
		"#808000",
		"#00ff00",
		"#008080",
		"#0000ff",
	})

	oklab, err := NewGradient().
		HtmlColors("#f00", "#0f0", "#00f").
		Mode(BlendOklab).
		Interpolation(InterpolationLinear).
		Build()
	test(t, err, nil)
	testSlice(t, colors2hex(oklab.Colors(5)), []string{
		"#ff0000",
		"#d0a801",
		"#00ff01",
		"#00aabf",
		"#0000ff",
	})

	// BlendOklch on the wrap-around sample goes through 0° while holding
	// lightness and chroma constant; BlendOklab's Cartesian interpolation
	// instead cuts a chord, dipping in chroma.
	wrap, err := NewGradient().
		Colors(
			mustOklch(0.65, 0.2, 350, 1),
			mustOklch(0.65, 0.2, 10, 1),
		).
		Mode(BlendOklch).
		Build()
	test(t, err, nil)
	lab, err := NewGradient().
		Colors(
			mustOklch(0.65, 0.2, 350, 1),
			mustOklch(0.65, 0.2, 10, 1),
		).
		Mode(BlendOklab).
		Build()
	test(t, err, nil)

	_, wrapC, wrapH := oklchComponents(wrap.At(0.5))
	_, labC, _ := oklchComponents(lab.At(0.5))
	testTrue(t, hueDistance(wrapH, 0) < 1.0)
	testTrue(t, math.Abs(wrapC-0.2) < 0.01)
	testTrue(t, labC < wrapC)
}

// --- helpers

func assertValidColors(t *testing.T, g Gradient, from, to float64, steps int) {
	t.Helper()
	for i := 0; i <= steps; i++ {
		p := from + (to-from)*float64(i)/float64(steps)
		c := g.At(p)
		valid := func(v float64) bool {
			return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= 1
		}
		if !valid(c.R) || !valid(c.G) || !valid(c.B) || !valid(c.A) {
			t.Errorf("invalid color at %v: %+v", p, c)
			return
		}
	}
}

// Spread helpers and Sharp() operate on top of At(), so they must keep working
// with BlendOklch gradients.
func Test_OklchSpreadAndSharp(t *testing.T) {
	grad, err := NewGradient().
		Colors(
			mustOklch(0.65, 0.2, 350, 1),
			mustOklch(0.65, 0.2, 10, 1),
		).
		Mode(BlendOklch).
		Build()
	test(t, err, nil)

	test(t, grad.RepeatAt(1.0), grad.At(0))
	test(t, grad.RepeatAt(0.5), grad.At(0.5))
	test(t, grad.RepeatAt(-0.5), grad.At(0.5))
	test(t, grad.ReflectAt(1.0), grad.At(1))
	test(t, grad.ReflectAt(-0.25), grad.At(0.25))

	sharp := grad.Sharp(3, 0)
	test(t, sharp.At(0).HexString(), grad.At(0).HexString())
	test(t, sharp.At(1).HexString(), grad.At(1).HexString())
	assertValidColors(t, sharp, 0, 1, 200)
}
