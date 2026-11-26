package colorgrad

import (
	"errors"
	"math"
	"testing"
)

func TestContrastRatio_BlackOnHalfGray(t *testing.T) {
	black := Rgb(0, 0, 0, 1)
	gray := Rgb(0.5, 0.5, 0.5, 1)

	ratio := ContrastRatio(black, gray)
	if math.Abs(ratio-5.280822809645) > 1e-9 {
		t.Fatalf("WCAG ratio = %.12f, want about 5.2808", ratio)
	}
	if math.Abs(ratio-5.28) > 0.01 {
		t.Fatalf("ratio %.4f is not approximately 5.28:1", ratio)
	}
	if r := ContrastRatio(Rgb(1, 1, 1, 1), black); math.Abs(r-21) > 1e-12 {
		t.Fatalf("black/white ratio = %.12f, want 21", r)
	}
}

func TestMinContrast_WhiteGrayWhiteFixedBar(t *testing.T) {
	text := Rgb(0, 0, 0, 1)
	base := Rgb(1, 1, 1, 1)
	gray := Rgb(0.5, 0.5, 0.5, 1)

	modes := []Interpolation{
		InterpolationLinear,
		InterpolationSmoothstep,
		InterpolationBasis,
		InterpolationCatmullRom,
	}

	for _, mode := range modes {
		g, err := NewGradient().
			Colors(Rgb(1, 1, 1, 1), gray, Rgb(1, 1, 1, 1)).
			Interpolation(mode).
			Build()
		if err != nil {
			t.Fatalf("mode %d build: %v", mode, err)
		}

		got, err := g.MinContrast(text, base)
		if err != nil {
			t.Fatalf("mode %d MinContrast: %v", mode, err)
		}
		if math.Abs(got.Position-0.5) > 1e-9 {
			t.Fatalf("mode %d position = %.17g, want center", mode, got.Position)
		}
		if got.Ratio >= ContrastRatio(text, base) {
			t.Fatalf("mode %d ratio %.6f unexpectedly not below white endpoint", mode, got.Ratio)
		}
		if math.Abs(got.Ratio-ContrastRatio(text, g.At(got.Position))) > 1e-10 {
			t.Fatalf("mode %d returned ratio does not match returned position", mode)
		}
	}

	// Linear, smoothstep and Catmull-Rom pass through the middle gray stop.
	for _, mode := range []Interpolation{InterpolationLinear, InterpolationSmoothstep, InterpolationCatmullRom} {
		g, _ := NewGradient().
			Colors(Rgb(1, 1, 1, 1), gray, Rgb(1, 1, 1, 1)).
			Interpolation(mode).
			Build()
		got, err := MinContrast(g, text, base)
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(got.Ratio-5.280822809645) > 1e-6 {
			t.Fatalf("mode %d ratio = %.9f, want 5.2808", mode, got.Ratio)
		}
	}

	// The hard-edge version has a centered gray band; any stable point in
	// that band is a valid minimum.
	linear, _ := NewGradient().
		Colors(Rgb(1, 1, 1, 1), gray, Rgb(1, 1, 1, 1)).
		Build()
	sharp := linear.Sharp(3, 0)
	got, err := sharp.MinContrast(text, base)
	if err != nil {
		t.Fatal(err)
	}
	if got.Position < 1.0/3.0-1e-9 || got.Position > 2.0/3.0+1e-9 {
		t.Fatalf("sharp position = %.12f, want middle gray band", got.Position)
	}
	if math.Abs(got.Ratio-5.280822809645) > 1e-8 {
		t.Fatalf("sharp ratio = %.9f, want 5.2808", got.Ratio)
	}
}

func TestMinContrast_InteriorMinimumNotAtEndpoints(t *testing.T) {
	// In sRGB space, red-to-green passes through a dark yellow/olive region
	// whose WCAG luminance is lower than either endpoint.
	g, err := NewGradient().HtmlColors("#f00", "#0f0").Build()
	if err != nil {
		t.Fatal(err)
	}
	text := Rgb(0, 0, 0, 1)
	base := Rgb(1, 1, 1, 1)

	got, err := MinContrast(g, text, base)
	if err != nil {
		t.Fatal(err)
	}
	if got.Position <= 0.1 || got.Position >= 0.5 {
		t.Fatalf("position = %.12f, want an interior minimum", got.Position)
	}
	if math.Abs(got.Position-0.273533) > 1e-5 {
		t.Fatalf("position = %.12f, want approximately 0.273533", got.Position)
	}
	endpointMin := math.Min(ContrastRatio(text, g.At(0)), ContrastRatio(text, g.At(1)))
	if got.Ratio >= endpointMin-0.5 {
		t.Fatalf("minimum %.6f should be materially below endpoint minimum %.6f", got.Ratio, endpointMin)
	}

	ratio, pos, err := g.MinContrastXY(text, base)
	if err != nil {
		t.Fatal(err)
	}
	if ratio != got.Ratio || pos != got.Position {
		t.Fatalf("XY helper = (%v,%v), want (%v,%v)", ratio, pos, got.Ratio, got.Position)
	}
}

func TestMinContrast_AlphaCompositing(t *testing.T) {
	black := Rgb(0, 0, 0, 1)
	white := Rgb(1, 1, 1, 1)

	// 50% opaque black gradient over opaque white becomes sRGB gray 0.5.
	g, err := NewGradient().
		Colors(Rgb(0, 0, 0, 0.5), Rgb(0, 0, 0, 0.5)).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	got, err := g.MinContrast(black, white)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got.Ratio-5.280822809645) > 1e-9 {
		t.Fatalf("gradient alpha ratio = %.12f, want 5.2808", got.Ratio)
	}

	// 50% opaque black text over white becomes gray 0.5; the backdrop stays
	// white, so the ratio is white/gray rather than black/gray.
	got, err = MinContrast(mustGradient(t, NewGradient().Colors(white, white)), Rgb(0, 0, 0, 0.5), white)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got.Ratio-3.976653024912) > 1e-9 {
		t.Fatalf("text alpha ratio = %.12f, want 3.97665", got.Ratio)
	}

	// Translucent text composites over the already flattened gradient, not
	// directly over the distant opaque base.
	halfBlackGradient := mustGradient(t, NewGradient().Colors(Rgb(0, 0, 0, 0.5), Rgb(0, 0, 0, 0.5)))
	got, err = halfBlackGradient.MinContrast(Rgb(0, 0, 0, 0.5), white)
	if err != nil {
		t.Fatal(err)
	}
	backdrop := compositeOver(Rgb(0, 0, 0, 0.5), white)
	foreground := compositeOver(Rgb(0, 0, 0, 0.5), backdrop)
	wantLayered := ContrastRatio(foreground, backdrop)
	if math.Abs(got.Ratio-wantLayered) > 1e-12 {
		t.Fatalf("layered alpha ratio = %.12f, want %.12f", got.Ratio, wantLayered)
	}
}

func TestMinContrast_DoesNotChangeAt(t *testing.T) {
	g, err := NewGradient().HtmlColors("#123456", "#abcdef", "#fedcba").Build()
	if err != nil {
		t.Fatal(err)
	}
	before := make([]Color, 0, 33)
	for i := 0; i <= 32; i++ {
		before = append(before, g.At(float64(i)/32))
	}
	if _, err := g.MinContrast(Rgb(0.1, 0.2, 0.3, 0.7), Rgb(1, 1, 1, 1)); err != nil {
		t.Fatal(err)
	}
	for i, want := range before {
		if got := g.At(float64(i) / 32); got != want {
			t.Fatalf("At(%d/32) changed: %+v != %+v", i, got, want)
		}
	}
}

func TestMinContrast_StableFlatRepeatedAndSymmetricTies(t *testing.T) {
	text := Rgb(0, 0, 0, 1)
	base := Rgb(1, 1, 1, 1)

	flat := Gradient{Core: constContrastCore{c: Rgb(0.2, 0.3, 0.4, 1)}, Min: -2, Max: 5}
	got, err := flat.MinContrast(text, base)
	if err != nil {
		t.Fatal(err)
	}
	if got.Position != -2 {
		t.Fatalf("flat position = %.17g, want domain min -2", got.Position)
	}
	again, _ := flat.MinContrast(text, base)
	if got != again {
		t.Fatalf("flat query not stable: %+v != %+v", got, again)
	}

	repeated, err := NewGradient().
		HtmlColors("#fff", "#fff", "#808080", "#808080").
		Build()
	if err != nil {
		t.Fatal(err)
	}
	got, err = repeated.MinContrast(text, base)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got.Position-2.0/3.0) > 1e-7 {
		t.Fatalf("repeated-color position = %.12f, want first gray stop", got.Position)
	}

	two := Gradient{Core: twoValleyCore{}, Min: 0, Max: 1}
	first, err := two.MinContrast(text, base)
	if err != nil {
		t.Fatal(err)
	}
	second, err := two.MinContrast(text, base)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("equal minima not deterministic: %+v vs %+v", first, second)
	}
	if math.Abs(first.Position-0.25) > 1e-7 {
		t.Fatalf("tie position = %.12f, want left valley 0.25", first.Position)
	}
}

func TestMinContrast_PrecisionAndCallBound(t *testing.T) {
	core := &countingContrastCore{inner: parabolaContrastCore{want: 0.371234567}}
	g := Gradient{Core: core, Min: 0, Max: 1}

	got, err := g.MinContrast(Rgb(0, 0, 0, 1), Rgb(1, 1, 1, 1))
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got.Position-0.371234567) > 1e-7 {
		t.Fatalf("position = %.12f, want 0.371234567", got.Position)
	}
	if core.calls > 3072 {
		t.Fatalf("At called %d times, want at most 3072", core.calls)
	}
	t.Logf("contrast search used %d At calls", core.calls)

	// Non-unit domains use domain coordinates, not normalized coordinates.
	offset := Gradient{Core: parabolaContrastCore{want: -3.25}, Min: -10, Max: 10}
	got, err = offset.MinContrast(Rgb(0, 0, 0, 1), Rgb(1, 1, 1, 1))
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got.Position+3.25) > 1e-6 {
		t.Fatalf("offset position = %.12f, want -3.25", got.Position)
	}
}

func TestMinContrast_InvalidInputs(t *testing.T) {
	valid, err := NewGradient().HtmlColors("#000", "#fff").Build()
	if err != nil {
		t.Fatal(err)
	}
	black := Rgb(0, 0, 0, 1)
	white := Rgb(1, 1, 1, 1)

	colors := map[string]Color{
		"nan":         Rgb(math.NaN(), 0, 0, 1),
		"inf":         Rgb(math.Inf(1), 0, 0, 1),
		"negative":    Rgb(-0.1, 0, 0, 1),
		"above-range": Rgb(1.1, 0, 0, 1),
		"alpha-range": Rgb(0, 0, 0, 1.1),
	}
	for name, c := range colors {
		if _, err := valid.MinContrast(c, white); !errors.Is(err, ErrContrastQuery) {
			t.Fatalf("text %s: err=%v, want ErrContrastQuery", name, err)
		}
		if _, err := valid.MinContrast(black, c); !errors.Is(err, ErrContrastQuery) {
			t.Fatalf("background %s: err=%v, want ErrContrastQuery", name, err)
		}
	}
	if _, err := valid.MinContrast(black, Rgb(1, 1, 1, 0)); !errors.Is(err, ErrInvalidColor) {
		t.Fatalf("transparent background err=%v, want ErrInvalidColor", err)
	}

	domains := map[string]Gradient{
		"nil core":       {Min: 0, Max: 1},
		"nan min":        {Core: valid.Core, Min: math.NaN(), Max: 1},
		"inf max":        {Core: valid.Core, Min: 0, Max: math.Inf(1)},
		"equal domain":   {Core: valid.Core, Min: 1, Max: 1},
		"reverse domain": {Core: valid.Core, Min: 2, Max: 1},
	}
	for name, g := range domains {
		if _, err := g.MinContrast(black, white); !errors.Is(err, ErrContrastQuery) {
			t.Fatalf("%s: err=%v, want ErrContrastQuery", name, err)
		}
	}

	badAt := Gradient{Core: nanContrastCore{}, Min: 0, Max: 1}
	if _, err := badAt.MinContrast(black, white); !errors.Is(err, ErrNonFiniteValue) {
		t.Fatalf("non-finite At err=%v, want ErrNonFiniteValue", err)
	}
}

func mustGradient(t *testing.T, b *GradientBuilder) Gradient {
	t.Helper()
	g, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	return g
}

type constContrastCore struct{ c Color }

func (c constContrastCore) At(float64) Color { return c.c }

type twoValleyCore struct{}

func (twoValleyCore) At(t float64) Color {
	d := math.Min(math.Abs(t-0.25), math.Abs(t-0.75))
	v := 0.4 + 0.6*d
	return Rgb(v, v, v, 1)
}

type parabolaContrastCore struct{ want float64 }

func (p parabolaContrastCore) At(t float64) Color {
	width := 1.0
	// The same shape is used on [-10, 10] by scaling the coefficient.
	if p.want < -1 {
		width = 20
	}
	scale := 8 / (width * width)
	v := 0.2 + scale*(t-p.want)*(t-p.want)
	return Rgb(v, v, v, 1)
}

type countingContrastCore struct {
	inner parabolaContrastCore
	calls int
}

func (c *countingContrastCore) At(t float64) Color {
	c.calls++
	return c.inner.At(t)
}

type nanContrastCore struct{}

func (nanContrastCore) At(float64) Color { return Rgb(math.NaN(), 0, 0, 1) }
