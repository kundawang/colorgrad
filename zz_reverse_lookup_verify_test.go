package colorgrad_test

import (
	"errors"
	"image/color"
	"math"
	"testing"

	colorgrad "github.com/mazznoer/colorgrad"
)

// These tests verify the reverse color lookup end to end through the exported
// API. All identifiers carry a ZZReverseLookup prefix and the file lives in
// the external _test package so it cannot collide with package-internal test
// files added separately.

func zzRLApprox(a, b, eps float64) bool { return math.Abs(a-b) <= eps }

func TestZZReverseLookup_RedBlueMidpoint(t *testing.T) {
	g, err := colorgrad.NewGradient().HtmlColors("#f00", "#00f").
		Mode(colorgrad.BlendRgb).Build()
	if err != nil {
		t.Fatal(err)
	}

	want := colorgrad.Rgb(0.5, 0, 0.5, 1)
	m, err := g.FindPosition(want, 1e-9)
	if err != nil {
		t.Fatal(err)
	}
	if !zzRLApprox(m.Position, 0.5, 1e-12) {
		t.Fatalf("position = %.17g, want 0.5", m.Position)
	}
	if m.Distance > 1e-12 {
		t.Fatalf("distance = %g, want ~0", m.Distance)
	}
	if !m.Matched || !m.Found || !m.OK {
		t.Fatalf("match flags not set: %+v", m)
	}
	if m.Color.HexString() != g.At(0.5).HexString() {
		t.Fatalf("returned color %s != At(0.5) %s", m.Color.HexString(), g.At(0.5).HexString())
	}
	if m.Pos != m.Position || m.T != m.Position || m.Error != m.Distance || m.Actual != m.Color {
		t.Fatalf("alias fields disagree: %+v", m)
	}

	// Endpoints map exactly to the domain ends.
	if m, _ := g.FindColor(colorgrad.Rgb(1, 0, 0, 1), 1e-9); m.Position != 0 {
		t.Fatalf("red at %v, want 0", m.Position)
	}
	if m, _ := g.FindColor(colorgrad.Rgb(0, 0, 1, 1), 1e-9); m.Position != 1 {
		t.Fatalf("blue at %v, want 1", m.Position)
	}
}

func TestZZReverseLookup_RepeatedColorReturnsEarlierPosition(t *testing.T) {
	g, err := colorgrad.NewGradient().HtmlColors("#f00", "#00f", "#f00").
		Mode(colorgrad.BlendRgb).Build()
	if err != nil {
		t.Fatal(err)
	}

	purple := colorgrad.Rgb(0.5, 0, 0.5, 1)
	m, err := colorgrad.FindPosition(g, purple, 1e-9) // package-level form
	if err != nil {
		t.Fatal(err)
	}
	// Purple appears at both 0.25 and 0.75; the earlier one must win.
	if !zzRLApprox(m.Position, 0.25, 1e-12) {
		t.Fatalf("position = %.17g, want 0.25 (leftmost of the two ties)", m.Position)
	}

	// Duplicated red at both ends still resolves to the first position.
	m, err = g.FindPosition(colorgrad.Rgb(1, 0, 0, 1), 1e-9)
	if err != nil {
		t.Fatal(err)
	}
	if m.Position != 0 {
		t.Fatalf("red position = %v, want 0", m.Position)
	}
}

func TestZZReverseLookup_CustomDomain(t *testing.T) {
	g, err := colorgrad.NewGradient().HtmlColors("#f00", "#00f").
		Domain(10, 30).Mode(colorgrad.BlendRgb).Build()
	if err != nil {
		t.Fatal(err)
	}
	m, err := g.FindColorPosition(colorgrad.Rgb(0.5, 0, 0.5, 1), 1e-9)
	if err != nil {
		t.Fatal(err)
	}
	if !zzRLApprox(m.Position, 20, 1e-10) {
		t.Fatalf("position = %v, want 20", m.Position)
	}
	if p, err := g.PositionOfColor(colorgrad.Rgb(0.5, 0, 0.5, 1), 1e-9); err != nil || !zzRLApprox(p, 20, 1e-10) {
		t.Fatalf("PositionOfColor = %v, %v", p, err)
	}
}

func TestZZReverseLookup_FlatAndDuplicateStopsPreferMin(t *testing.T) {
	g, err := colorgrad.NewGradient().HtmlColors("#808080", "#808080").Build()
	if err != nil {
		t.Fatal(err)
	}
	m, err := g.FindColor(colorgrad.Rgb(0x80/255.0, 0x80/255.0, 0x80/255.0, 1), 1e-9)
	if err != nil {
		t.Fatal(err)
	}
	if m.Position != g.Min {
		t.Fatalf("constant gradient position = %v, want domain min %v", m.Position, g.Min)
	}

	// Two identical red stops create a flat red band over the first half.
	g2, err := colorgrad.NewGradient().HtmlColors("#f00", "#f00", "#00f").
		Mode(colorgrad.BlendRgb).Build()
	if err != nil {
		t.Fatal(err)
	}
	m2, err := g2.FindColor(colorgrad.Rgb(1, 0, 0, 1), 1e-9)
	if err != nil {
		t.Fatal(err)
	}
	if m2.Position != 0 {
		t.Fatalf("flat red band position = %v, want 0", m2.Position)
	}
	if m3, _ := g2.FindColor(colorgrad.Rgb(0.5, 0, 0.5, 1), 1e-9); !zzRLApprox(m3.Position, 0.75, 1e-9) {
		t.Fatalf("second-half purple position = %v, want 0.75", m3.Position)
	}
}

func TestZZReverseLookup_AllInterpolations(t *testing.T) {
	for _, ip := range []colorgrad.Interpolation{
		colorgrad.InterpolationLinear,
		colorgrad.InterpolationSmoothstep,
		colorgrad.InterpolationBasis,
		colorgrad.InterpolationCatmullRom,
	} {
		g, err := colorgrad.NewGradient().HtmlColors("#000", "#fff").Interpolation(ip).Build()
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range []float64{0.1, 0.25, 0.5, 0.75, 0.9} {
			target := g.At(p)
			m, err := g.FindPosition(target, 1e-5)
			if err != nil {
				t.Fatalf("interp %d: %v", ip, err)
			}
			if m.Position < g.Min || m.Position > g.Max {
				t.Fatalf("interp %d: position %v outside domain", ip, m.Position)
			}
			if m.Color.HexString() != g.At(m.Position).HexString() {
				t.Fatalf("interp %d: color inconsistent", ip)
			}
			if m.Distance > 1e-4 {
				t.Fatalf("interp %d p=%v distance=%g", ip, p, m.Distance)
			}
		}
	}
}

func TestZZReverseLookup_Presets(t *testing.T) {
	for name, g := range map[string]colorgrad.Gradient{
		"Rainbow":  colorgrad.Rainbow(),
		"Turbo":    colorgrad.Turbo(),
		"Viridis":  colorgrad.Viridis(),
		"Sinebow":  colorgrad.Sinebow(),
		"Greys":    colorgrad.Greys(),
		"Cividis":  colorgrad.Cividis(),
		"Spectral": colorgrad.Spectral(),
	} {
		for _, p := range []float64{0.05, 0.25, 0.5, 0.75, 0.95} {
			target := g.At(p)
			m, err := g.FindColor(target, 0.02)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if m.Position < 0 || m.Position > 1 {
				t.Fatalf("%s: position %v outside [0,1]", name, m.Position)
			}
			if m.Color.HexString() != g.At(m.Position).HexString() {
				t.Fatalf("%s: returned color is not At(position)", name)
			}
			if m.Distance > 0.02 {
				t.Fatalf("%s p=%v distance=%g", name, p, m.Distance)
			}
		}
	}
}

func TestZZReverseLookup_Sharp(t *testing.T) {
	base, err := colorgrad.NewGradient().HtmlColors("#f00", "#0f0", "#00f").Build()
	if err != nil {
		t.Fatal(err)
	}
	g := base.Sharp(3, 0)
	for _, tc := range []struct {
		col colorgrad.Color
		pos float64
	}{
		{colorgrad.Rgb(1, 0, 0, 1), 0},
		{colorgrad.Rgb(0, 1, 0, 1), 1.0 / 3.0},
		{colorgrad.Rgb(0, 0, 1, 1), 2.0 / 3.0},
	} {
		m, err := g.FindColor(tc.col, 1e-9)
		if err != nil {
			t.Fatal(err)
		}
		if !m.Matched {
			t.Fatalf("sharp band %v not matched, distance %g", tc.pos, m.Distance)
		}
		if !zzRLApprox(m.Position, tc.pos, 1e-6) {
			t.Fatalf("sharp band position = %v, want ~%v", m.Position, tc.pos)
		}
	}
}

func TestZZReverseLookup_NoMatchStillReturnsClosest(t *testing.T) {
	g, _ := colorgrad.NewGradient().HtmlColors("#f00", "#00f").Build()
	m, err := g.FindColor(colorgrad.Rgb(0, 1, 0, 1), 0.01)
	if err != nil {
		t.Fatal(err)
	}
	if m.Matched {
		t.Fatalf("green should not match red-blue ramp at tol .01, distance %g", m.Distance)
	}
	if m.Position < 0 || m.Position > 1 {
		t.Fatalf("position outside domain: %v", m.Position)
	}
	if m.Color.HexString() != g.At(m.Position).HexString() {
		t.Fatal("color inconsistent")
	}

	// The strict form reports the miss with ErrNoMatch while returning the
	// closest match.
	sm, err := g.FindWithinTolerance(colorgrad.Rgb(0, 1, 0, 1), 0.01)
	if !errors.Is(err, colorgrad.ErrNoMatch) {
		t.Fatalf("strict err = %v, want ErrNoMatch", err)
	}
	if sm.Position != m.Position {
		t.Fatal("strict result should carry the closest match")
	}
	if _, err := g.FindWithinTolerance(colorgrad.Rgb(1, 0, 0, 1), 1e-9); err != nil {
		t.Fatalf("strict exact match: %v", err)
	}
}

func TestZZReverseLookup_InvalidInputs(t *testing.T) {
	g, _ := colorgrad.NewGradient().HtmlColors("#f00", "#00f").Build()
	var zero colorgrad.PositionMatch

	for _, tol := range []float64{-1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		m, err := g.FindColor(colorgrad.Rgb(0.5, 0, 0.5, 1), tol)
		if !errors.Is(err, colorgrad.ErrInvalidTolerance) {
			t.Fatalf("tol %v: err %v", tol, err)
		}
		if m != zero {
			t.Fatalf("tol %v: result not zero: %+v", tol, m)
		}
	}

	for _, c := range []colorgrad.Color{
		{R: math.NaN(), A: 1},
		{G: math.Inf(1), A: 1},
		{B: math.Inf(-1), A: 1},
		{A: math.NaN()},
	} {
		m, err := g.FindColor(c, 0.1)
		if !errors.Is(err, colorgrad.ErrNonFiniteColor) || !errors.Is(err, colorgrad.ErrInvalidColor) {
			t.Fatalf("color %+v: err %v", c, err)
		}
		if m != zero {
			t.Fatalf("color %+v: result not zero", c)
		}
	}

	for _, bad := range []colorgrad.Gradient{
		{},
		{Core: g.Core, Min: 1, Max: 1},
		{Core: g.Core, Min: 2, Max: 1},
		{Core: g.Core, Min: math.NaN(), Max: 1},
		{Core: g.Core, Min: 0, Max: math.Inf(1)},
	} {
		m, err := bad.FindColor(colorgrad.Rgb(0.5, 0, 0.5, 1), 0.1)
		if !errors.Is(err, colorgrad.ErrInvalidDomain) || !errors.Is(err, colorgrad.ErrEmptyDomain) {
			t.Fatalf("domain %+v: err %v", bad, err)
		}
		if m != zero {
			t.Fatalf("domain %+v: result not zero", bad)
		}
	}

	// Nil target color is reported as an invalid color rather than panicking.
	if _, err := g.FindColor(nil, 0.1); !errors.Is(err, colorgrad.ErrNonFiniteColor) {
		t.Fatalf("nil target err = %v", err)
	}
}

// zzRLCountingCore counts At calls while delegating to an inner core.
type zzRLCountingCore struct {
	inner colorgrad.GradientCore
	calls int
}

func (c *zzRLCountingCore) At(t float64) colorgrad.Color {
	c.calls++
	return c.inner.At(t)
}

func TestZZReverseLookup_BoundedCost(t *testing.T) {
	rng := func(seed int64) func() float64 {
		s := uint64(seed)
		return func() float64 {
			s = s*6364136223846793005 + 1442695040888963407
			return float64((s>>11)&((1<<53)-1)) / float64(uint64(1)<<53)
		}
	}

	for _, n := range []int{3, 2000} {
		rnd := rng(int64(n))
		cols := make([]colorgrad.Color, n)
		for i := range cols {
			cols[i] = colorgrad.Rgb(rnd(), rnd(), rnd(), 1)
		}
		g, err := colorgrad.NewGradient().Colors(cols...).
			Interpolation(colorgrad.InterpolationCatmullRom).Build()
		if err != nil {
			t.Fatal(err)
		}
		core := &zzRLCountingCore{inner: g.Core}
		wrapped := colorgrad.Gradient{Core: core, Min: g.Min, Max: g.Max}

		if _, err := wrapped.FindColor(colorgrad.Rgb(0.3, 0.6, 0.2, 1), 0.1); err != nil {
			t.Fatal(err)
		}
		// The lookup has a hard 4096-evaluation ceiling.
		if core.calls > 4096 {
			t.Fatalf("n=%d calls=%d exceed hard bound", n, core.calls)
		}
	}
}

func TestZZReverseLookup_AtUnchanged(t *testing.T) {
	g, _ := colorgrad.NewGradient().HtmlColors("#f00", "#0f0", "#00f").
		Domain(-3, 7).Mode(colorgrad.BlendOklab).Build()

	before := make([]string, 41)
	for i := range before {
		before[i] = g.At(-3 + float64(i)*0.25).HexString()
	}
	for i := 0; i < 41; i++ {
		if _, err := g.FindPosition(g.At(-3+float64(i)*0.25), 1e-3); err != nil {
			t.Fatal(err)
		}
	}
	for i := range before {
		if got := g.At(-3 + float64(i)*0.25).HexString(); got != before[i] {
			t.Fatalf("At changed at index %d: %s != %s", i, got, before[i])
		}
	}
}

func TestZZReverseLookup_StdlibRGBAColor(t *testing.T) {
	g, _ := colorgrad.NewGradient().HtmlColors("#f00", "#00f").Build()
	// 8-bit purple (128,0,128) maps to ~0.502 on the float ramp.
	m, err := g.FindColor(color.RGBA{R: 128, B: 128, A: 255}, 0.02)
	if err != nil {
		t.Fatal(err)
	}
	if !zzRLApprox(m.Position, 0.502, 0.01) {
		t.Fatalf("rgba position = %v", m.Position)
	}

	// The standard color.Color interface carrying a repo Color also works.
	var c color.Color = colorgrad.Rgb(0.5, 0, 0.5, 1)
	m2, err := g.ReverseLookup(c, 1e-9)
	if err != nil || !zzRLApprox(m2.Position, 0.5, 1e-12) {
		t.Fatalf("interface target: %+v err=%v", m2, err)
	}
}
