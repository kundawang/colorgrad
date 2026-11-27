package colorgrad

import (
	"errors"
	"fmt"
	"math"
	"sort"
)

const (
	// paletteSampleIntervals is the number of uniform parameter intervals used
	// to approximate the perceptual arc length of a gradient.
	paletteSampleIntervals = 2048

	// paletteFlatRegularizer is the fraction of the total perceptual length
	// added back as a parameter-proportional term. It keeps positions ordered
	// inside flat regions without measurably changing the spacing of
	// gradients that do change.
	paletteFlatRegularizer = 1e-9

	// paletteMinLength is the total perceptual length below which a gradient
	// is considered constant. Such gradients fall back to evenly spaced
	// positions instead of amplifying floating-point noise.
	paletteMinLength = 1e-12
)

// Errors returned by PerceptualPalette. Callers should use errors.Is:
//
//	palette, err := grad.PerceptualPalette(9, 1)
//	if errors.Is(err, colorgrad.ErrPaletteAlphaWeight) { ... }
var (
	// ErrPaletteRequest is the umbrella error matched by every invalid
	// perceptual palette request.
	ErrPaletteRequest = errors.New("colorgrad: invalid perceptual palette request")

	// ErrPaletteAlphaWeight is returned when the alpha weight is negative,
	// NaN or infinite.
	ErrPaletteAlphaWeight = fmt.Errorf("%w: alpha weight must be finite and non-negative", ErrPaletteRequest)

	// ErrPaletteDomain is returned when the gradient has no core or when its
	// domain is empty, reversed or not finite.
	ErrPaletteDomain = fmt.Errorf("%w: gradient domain must be finite with max greater than min", ErrPaletteRequest)

	// ErrPaletteColor is returned when the gradient produces a non-finite
	// color. No partially filled palette is returned in that case.
	ErrPaletteColor = fmt.Errorf("%w: gradient produced a non-finite color", ErrPaletteRequest)
)

// PaletteEntry is one sample of a perceptually spaced palette.
type PaletteEntry struct {
	// Color is the gradient color at Position, clamped to the displayable
	// range (each channel in 0..1).
	Color Color

	// Position is the sample position in the gradient's domain.
	Position float64

	// Pos is an alias for Position.
	Pos float64

	// T is an alias for Position.
	T float64
}

// PaletteColor is an alias for PaletteEntry.
type PaletteColor = PaletteEntry

func newPaletteEntry(c Color, position float64) PaletteEntry {
	return PaletteEntry{Color: c, Position: position, Pos: position, T: position}
}

// PerceptualPalette returns count colors sampled at equal perceptual distance
// along the gradient, paired with their positions in the gradient's domain.
//
// Unlike Colors, which advances through the parameter space in equal steps,
// PerceptualPalette advances by color change. Regions where the color changes
// quickly receive proportionally more samples, and nearly flat regions receive
// fewer. This is useful for discrete color bands, such as choropleth maps,
// where Colors would bunch most bands into a small part of the color range.
//
// The first entry is always exactly at the domain minimum and the last exactly
// at the domain maximum. Positions are returned in non-decreasing order, so
// the result is stable for a given gradient and arguments.
//
// Perceptual distance is the Euclidean distance in Oklab, with the alpha
// difference scaled by alphaWeight: 0 ignores alpha, and 1 treats a full alpha
// change on the same scale as a full Oklab lightness change.
//
// A negative, NaN or infinite alphaWeight is rejected. A count of 0 returns an
// empty slice and a count of 1 returns the domain minimum, matching Colors.
// When the whole gradient has zero perceptual length (for example a constant
// color), the positions fall back to being evenly spaced so the result stays
// deterministic. Hard edges and repeated colors are sampled like any other
// part of the curve; their color jumps contribute their full perceptual
// length at the edge.
func (g Gradient) PerceptualPalette(count uint, alphaWeight float64) ([]PaletteEntry, error) {
	return perceptualPalette(g, count, alphaWeight)
}

// Palette is an alias for PerceptualPalette.
func (g Gradient) Palette(count uint, alphaWeight float64) ([]PaletteEntry, error) {
	return perceptualPalette(g, count, alphaWeight)
}

// PerceptualPalette is the package-function form of
// Gradient.PerceptualPalette.
func PerceptualPalette(g Gradient, count uint, alphaWeight float64) ([]PaletteEntry, error) {
	return perceptualPalette(g, count, alphaWeight)
}

// Palette is the package-function form of Gradient.PerceptualPalette.
func Palette(g Gradient, count uint, alphaWeight float64) ([]PaletteEntry, error) {
	return perceptualPalette(g, count, alphaWeight)
}

// PerceptualPaletteXY is a convenience form of PerceptualPalette that returns
// colors and positions as parallel slices.
func (g Gradient) PerceptualPaletteXY(count uint, alphaWeight float64) (colors []Color, positions []float64, err error) {
	entries, err := perceptualPalette(g, count, alphaWeight)
	if err != nil {
		return nil, nil, err
	}
	colors = make([]Color, len(entries))
	positions = make([]float64, len(entries))
	for i, e := range entries {
		colors[i] = e.Color
		positions[i] = e.Position
	}
	return colors, positions, nil
}

// PaletteXY is an alias for PerceptualPaletteXY.
func (g Gradient) PaletteXY(count uint, alphaWeight float64) (colors []Color, positions []float64, err error) {
	return g.PerceptualPaletteXY(count, alphaWeight)
}

// PerceptualColors is an alias for PerceptualPaletteXY on the method form.
func (g Gradient) PerceptualColors(count uint, alphaWeight float64) (colors []Color, positions []float64, err error) {
	return g.PerceptualPaletteXY(count, alphaWeight)
}

// PerceptualColors is the package-function form of
// Gradient.PerceptualPaletteXY: it returns colors and their domain positions
// as parallel slices.
func PerceptualColors(g Gradient, count uint, alphaWeight float64) (colors []Color, positions []float64, err error) {
	return g.PerceptualPaletteXY(count, alphaWeight)
}

// PerceptualPaletteXY is the package-function form of
// Gradient.PerceptualPaletteXY.
func PerceptualPaletteXY(g Gradient, count uint, alphaWeight float64) (colors []Color, positions []float64, err error) {
	return g.PerceptualPaletteXY(count, alphaWeight)
}

func perceptualPalette(g Gradient, count uint, alphaWeight float64) ([]PaletteEntry, error) {
	if math.IsNaN(alphaWeight) || math.IsInf(alphaWeight, 0) || alphaWeight < 0 {
		return nil, ErrPaletteAlphaWeight
	}
	if count == 0 {
		return []PaletteEntry{}, nil
	}
	if g.Core == nil || !paletteFinite(g.Min) || !paletteFinite(g.Max) {
		return nil, ErrPaletteDomain
	}
	width := g.Max - g.Min
	if !paletteFinite(width) || width <= 0 {
		return nil, ErrPaletteDomain
	}
	if count == 1 {
		// Matches Colors(1), which samples the domain minimum.
		return uniformPaletteEntries(g, count)
	}

	positions := paletteSamplePositions(g)

	// Measure the polyline length of the actual gradient curve in perceptual
	// space. Including the gradient's own stop positions and the point just
	// after each stop keeps hard edges sharp in the arc-length table.
	colors := make([]Color, len(positions))
	for i, t := range positions {
		c := g.At(t).Clamp()
		if !paletteFiniteColor(c) {
			return nil, fmt.Errorf("%w at position %v", ErrPaletteColor, t)
		}
		colors[i] = c
	}

	lengths := make([]float64, len(positions))
	for i := 1; i < len(positions); i++ {
		lengths[i] = lengths[i-1] + paletteColorDistance(colors[i-1], colors[i], alphaWeight)
	}
	total := lengths[len(lengths)-1]
	if !paletteFinite(total) {
		return nil, fmt.Errorf("%w: non-finite total color difference", ErrPaletteColor)
	}

	if total <= paletteMinLength {
		return uniformPaletteEntries(g, count)
	}

	// Add a tiny parameter-proportional term so that samples falling inside a
	// flat region still get distinct positions. Across the whole domain this
	// term is only paletteFlatRegularizer of the perceptual length, so it has
	// no visible effect on the spacing of changing regions.
	cumulative := make([]float64, len(positions))
	for i := 1; i < len(positions); i++ {
		segment := lengths[i] - lengths[i-1]
		regularizer := total * paletteFlatRegularizer * ((positions[i] - positions[i-1]) / width)
		cumulative[i] = cumulative[i-1] + segment + regularizer
	}
	totalCumulative := cumulative[len(cumulative)-1]
	if !paletteFinite(totalCumulative) || totalCumulative <= 0 {
		return nil, fmt.Errorf("%w: non-finite total color difference", ErrPaletteColor)
	}

	entries := make([]PaletteEntry, count)
	last := float64(count - 1)
	for i := range entries {
		target := totalCumulative * (float64(i) / last)
		t := palettePositionAt(positions, cumulative, target)
		// Interpolation is a convex combination, but floating-point rounding
		// on very narrow domains can land an ulp outside the domain.
		if t < g.Min {
			t = g.Min
		} else if t > g.Max {
			t = g.Max
		}
		c := g.At(t).Clamp()
		if !paletteFiniteColor(c) {
			return nil, fmt.Errorf("%w at position %v", ErrPaletteColor, t)
		}
		entries[i] = newPaletteEntry(c, t)
	}

	// Pin the ends exactly, independent of floating-point interpolation.
	entries[0] = newPaletteEntry(entries[0].Color, g.Min)
	entries[len(entries)-1] = newPaletteEntry(entries[len(entries)-1].Color, g.Max)
	return entries, nil
}

// uniformPaletteEntries is the deterministic fallback for a gradient whose
// total perceptual length is zero.
func uniformPaletteEntries(g Gradient, count uint) ([]PaletteEntry, error) {
	positions := linspace(g.Min, g.Max, count)
	entries := make([]PaletteEntry, len(positions))
	for i, t := range positions {
		c := g.At(t).Clamp()
		if !paletteFiniteColor(c) {
			return nil, fmt.Errorf("%w at position %v", ErrPaletteColor, t)
		}
		entries[i] = newPaletteEntry(c, t)
	}
	return entries, nil
}

// paletteSamplePositions returns the sorted parameter values at which the
// gradient is evaluated to approximate its perceptual arc length. It combines
// a uniform grid with the gradient's own stop positions and points just either
// side of them, so discontinuous gradients are captured accurately.
func paletteSamplePositions(g Gradient) []float64 {
	dmin, dmax := g.Min, g.Max
	width := dmax - dmin
	seen := make(map[float64]struct{}, paletteSampleIntervals+64)

	add := func(t float64) {
		if !paletteFinite(t) || t < dmin || t > dmax {
			return
		}
		seen[t] = struct{}{}
	}

	add(dmin)
	add(dmax)
	for i := 0; i <= paletteSampleIntervals; i++ {
		add(dmin + (float64(i)/paletteSampleIntervals)*width)
	}

	for _, p := range paletteBreakpoints(g.Core) {
		add(p)
		if p < dmax {
			add(math.Nextafter(p, math.Inf(1)))
		}
		if p > dmin {
			add(math.Nextafter(p, math.Inf(-1)))
		}
	}

	positions := make([]float64, 0, len(seen))
	for t := range seen {
		positions = append(positions, t)
	}
	sort.Float64s(positions)
	return positions
}

// paletteBreakpoints reports the parameter values at which the concrete
// gradient implementations can change their behavior. Unknown GradientCore
// implementations simply return nil and are sampled on the uniform grid.
func paletteBreakpoints(core GradientCore) []float64 {
	switch c := core.(type) {
	case linearGradient:
		return c.positions
	case *linearGradient:
		return c.positions
	case smoothstepGradient:
		return c.positions
	case *smoothstepGradient:
		return c.positions
	case basisGradient:
		return c.positions
	case *basisGradient:
		return c.positions
	case catmullRomGradient:
		return c.positions
	case *catmullRomGradient:
		return c.positions
	case sharpGradient:
		return c.positions
	case *sharpGradient:
		return c.positions
	case gimpGradient:
		return gimpBreakpoints(c)
	case *gimpGradient:
		return gimpBreakpoints(*c)
	default:
		return nil
	}
}

func gimpBreakpoints(g gimpGradient) []float64 {
	positions := make([]float64, 0, len(g.segments)*3)
	for _, s := range g.segments {
		positions = append(positions, s.lpos, s.mpos, s.rpos)
	}
	return positions
}

// palettePositionAt maps a target perceptual length to a parameter value by
// linearly interpolating inside the bracketing arc-length samples.
func palettePositionAt(positions, cumulative []float64, target float64) float64 {
	i := sort.SearchFloat64s(cumulative, target)
	switch {
	case i <= 0:
		return positions[0]
	case i >= len(cumulative):
		return positions[len(positions)-1]
	}

	lo, hi := cumulative[i-1], cumulative[i]
	if !(hi > lo) {
		return positions[i]
	}
	f := (target - lo) / (hi - lo)
	if f < 0 {
		f = 0
	} else if f > 1 {
		f = 1
	}
	if math.IsNaN(f) {
		return positions[i]
	}
	return positions[i-1] + f*(positions[i]-positions[i-1])
}

// ColorDistance returns the perceptual distance between two colors as the
// Euclidean distance in Oklab. Alpha is ignored; use ColorDistanceWeighted to
// include it.
func ColorDistance(a, b Color) float64 {
	return paletteColorDistance(a, b, 0)
}

// PerceptualDistance is an alias for ColorDistance.
func PerceptualDistance(a, b Color) float64 {
	return ColorDistance(a, b)
}

// ColorDistanceWeighted returns the perceptual distance between two colors as
// the Euclidean distance in Oklab, extended with the alpha difference scaled
// by alphaWeight. A weight of 0 ignores alpha.
func ColorDistanceWeighted(a, b Color, alphaWeight float64) float64 {
	return paletteColorDistance(a, b, alphaWeight)
}

// paletteColorDistance is the alpha-weighted Euclidean distance in Oklab.
func paletteColorDistance(a, b Color, alphaWeight float64) float64 {
	la, lb := a.Oklab(), b.Oklab()
	d := math.Hypot(la[0]-lb[0], la[1]-lb[1])
	d = math.Hypot(d, la[2]-lb[2])
	if alphaWeight > 0 {
		d = math.Hypot(d, alphaWeight*(a.A-b.A))
	}
	return d
}

func paletteFinite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

func paletteFiniteColor(c Color) bool {
	return paletteFinite(c.R) && paletteFinite(c.G) && paletteFinite(c.B) && paletteFinite(c.A)
}
