package colorgrad

import (
	"math"
	"testing"
)

func testClose(t *testing.T, a, b float64) {
	if math.Abs(a-b) > 1e-9 {
		t.Helper()
		t.Errorf("left: %v, right: %v", a, b)
	}
}

// A three-stop red peak moving from 20% to 80%: at t=0.5 the peak must sit
// at 50% with full opacity, not appear as two half-transparent peaks.
func Test_LerpGradient_RedPeak(t *testing.T) {
	peak := func(pos float64) Gradient {
		g, err := NewGradient().
			Colors(Rgb(1, 0, 0, 0), Rgb(1, 0, 0, 1), Rgb(1, 0, 0, 0)).
			Domain(pos-0.1, pos, pos+0.1).
			Build()
		if err != nil {
			t.Fatal(err)
		}
		return g
	}

	a := peak(0.2)
	b := peak(0.8)

	mid, err := LerpGradient(a, b, 0.5)
	test(t, err, nil)

	// stop positions moved to [0.4, 0.5, 0.6]
	dmin, dmax := mid.Domain()
	testClose(t, dmin, 0.4)
	testClose(t, dmax, 0.6)

	// single full-opacity peak at 50%
	test(t, mid.At(0.5).HexString(), "#ff0000")

	// no ghost peaks at the old/new positions
	test(t, mid.At(0.2).A, 0.0)
	test(t, mid.At(0.8).A, 0.0)

	// halfway up the moved peak
	test(t, mid.At(0.45).A, 0.5)
	test(t, mid.At(0.45).HexString(), "#ff000080")

	// weight 0 / 1 exactly equal the inputs
	g0, err := LerpGradient(a, b, 0)
	test(t, err, nil)
	testSlice(t, colors2hex(g0.Colors(9)), colors2hex(a.Colors(9)))

	g1, err := LerpGradient(a, b, 1)
	test(t, err, nil)
	testSlice(t, colors2hex(g1.Colors(9)), colors2hex(b.Colors(9)))
}

func Test_LerpGradient_Semantics(t *testing.T) {
	// Non-uniform domain: the resulting domain is the weighted mix.
	a, _ := NewGradient().HtmlColors("#000", "#fff").Domain(10, 20).Build()
	b, _ := NewGradient().HtmlColors("#000", "#fff").Domain(10, 30).Build()
	m, err := LerpGradient(a, b, 0.5)
	test(t, err, nil)
	dmin, dmax := m.Domain()
	test(t, dmin, 10.0)
	test(t, dmax, 25.0)
	test(t, m.At(17.5).HexString(), "#808080")

	// Hard edges (duplicated stop positions) stay hard and move.
	ha, _ := NewGradient().HtmlColors("#f00", "#f00", "#00f", "#00f").Domain(0, 0.5, 0.5, 1).Build()
	hb, _ := NewGradient().HtmlColors("#f00", "#f00", "#00f", "#00f").Domain(0, 0.25, 0.25, 1).Build()
	hm, err := LerpGradient(ha, hb, 0.5)
	test(t, err, nil)
	test(t, hm.At(0.374).HexString(), "#ff0000")
	test(t, hm.At(0.376).HexString(), "#0000ff")

	// Transparent stops: alpha is interpolated alongside the color.
	ta, _ := NewGradient().Colors(Rgb(1, 0, 0, 0), Rgb(1, 0, 0, 1)).Build()
	tb, _ := NewGradient().Colors(Rgb(1, 0, 0, 1), Rgb(1, 0, 0, 0)).Build()
	tm, err := LerpGradient(ta, tb, 0.5)
	test(t, err, nil)
	test(t, tm.At(0).A, 0.5)
	test(t, tm.At(1).A, 0.5)

	// Other blend modes: stops are interpolated in the mode's color space,
	// and weight 0 / 1 still reproduce the inputs exactly.
	oa, _ := NewGradient().HtmlColors("#f00", "#00f").Mode(BlendOklab).Build()
	ob, _ := NewGradient().HtmlColors("#0f0", "#00f").Mode(BlendOklab).Domain(0.2, 0.8).Build()
	om, err := LerpGradient(oa, ob, 0.5)
	test(t, err, nil)
	dmin, dmax = om.Domain()
	test(t, dmin, 0.1)
	test(t, dmax, 0.9)

	g0, err := LerpGradient(oa, ob, 0)
	test(t, err, nil)
	testSlice(t, colors2hex(g0.Colors(7)), colors2hex(oa.Colors(7)))

	g1, err := LerpGradient(oa, ob, 1)
	test(t, err, nil)
	testSlice(t, colors2hex(g1.Colors(7)), colors2hex(ob.Colors(7)))
}

func Test_LerpGradient_Errors(t *testing.T) {
	two, _ := NewGradient().HtmlColors("#f00", "#00f").Build()
	three, _ := NewGradient().HtmlColors("#f00", "#0f0", "#00f").Build()

	before := colors2hex(two.Colors(5))

	// different stop counts
	_, err := LerpGradient(two, three, 0.5)
	testTrue(t, err != nil)

	// invalid weights
	for _, w := range []float64{-0.1, 1.1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		_, err = LerpGradient(two, two, w)
		testTrue(t, err != nil)
	}

	// mismatched blend modes
	oklab, _ := NewGradient().HtmlColors("#f00", "#00f").Mode(BlendOklab).Build()
	_, err = LerpGradient(two, oklab, 0.5)
	testTrue(t, err != nil)

	// non-linear gradient
	sharp := two.Sharp(5, 0)
	_, err = LerpGradient(two, sharp, 0.5)
	testTrue(t, err != nil)

	// unsorted stop positions
	bad := Gradient{
		Core: linearGradient{
			colors:    [][4]float64{{1, 0, 0, 1}, {0, 0, 1, 1}},
			positions: []float64{0.8, 0.2},
			min:       0.8,
			max:       0.2,
			mode:      BlendRgb,
		},
		Min: 0.8,
		Max: 0.2,
	}
	_, err = LerpGradient(bad, two, 0.5)
	testTrue(t, err != nil)

	// failed transitions return a usable zero gradient
	z, err := LerpGradient(two, three, 0.5)
	testTrue(t, err != nil)
	testTrue(t, isZeroGradient(z))

	// the inputs are unchanged
	testSlice(t, colors2hex(two.Colors(5)), before)
}
