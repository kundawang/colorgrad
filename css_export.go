package colorgrad

import (
	"fmt"
	"math"
	"strconv"
)

// cssGridRes is the fixed sampling resolution used for adaptive
// approximation. Both the subdivision and the round-trip verification run
// on this deterministic grid, so the accepted output is guaranteed within
// tolerance at every grid point and the result does not depend on map
// iteration order or other nondeterminism.
const cssGridRes = 8192

// CssStops exports the gradient as CSS color stop text, using the same
// syntax accepted by [GradientBuilder.Css], for example:
//
//	"rgb(100% 0% 0%) 0%, rgb(0% 0% 100%) 100%"
//
// Exact, lossless export is only available for linear gradients in RGB
// blending mode (BlendRgb + InterpolationLinear). Stop positions, colors and
// same-position hard edges are all preserved. Any finite gradient domain is
// mapped to percentages, so a gradient built on a custom domain such as
// [-10, 20] is rebuilt by [GradientBuilder.Css] on the default [0, 1]
// domain; sampling it at normalized positions reproduces the original
// colors.
//
// Transparent colors are preserved through "rgb(r% g% b% / a)" syntax. An
// error is returned for unsupported gradient types, degenerate or
// non-finite domains, and non-finite color values. For any other gradient
// type use [Gradient.CssStopsApprox].
func (g Gradient) CssStops() (string, error) {
	min, max := g.Min, g.Max
	if !isFinite(min) || !isFinite(max) {
		return "", fmt.Errorf("CssStops: non-finite gradient domain: %v, %v", min, max)
	}
	if min >= max {
		return "", fmt.Errorf("CssStops: degenerate gradient domain: %v, %v", min, max)
	}

	lg, ok := g.Core.(linearGradient)
	if !ok || lg.mode != BlendRgb {
		return "", fmt.Errorf("CssStops: exact CSS export is only supported for linear gradients in BlendRgb mode, use CssStopsApprox instead")
	}

	stops := make([]cssStopSample, 0, len(lg.positions))
	for i, pos := range lg.positions {
		if !isFinite(pos) {
			return "", fmt.Errorf("CssStops: non-finite stop position: %v", pos)
		}
		c := lg.colors[i]
		col := Color{R: c[0], G: c[1], B: c[2], A: c[3]}
		if !colorIsFinite(col) {
			return "", fmt.Errorf("CssStops: non-finite color value: %v", col)
		}
		u := clamp01(norm(pos, min, max))
		stops = append(stops, cssStopSample{u: u, c: col})
	}

	return formatCssStops(stops), nil
}

// CssStopsApprox exports any gradient as CSS color stop text, using the
// same syntax accepted by [GradientBuilder.Css]. The gradient is sampled on
// a fixed 8192-point grid and adaptively subdivided into a piecewise-linear
// RGB approximation with at most maxStops stops, such that every RGBA
// channel of the rebuilt gradient stays within tolerance of the original
// curve at every grid point across the whole domain. Discontinuities (hard
// edges) are detected and emitted as two stops sharing the same position.
//
// tolerance is the maximum allowed absolute difference per RGBA channel,
// each channel being in the range [0, 1]; it must be finite and
// non-negative. maxStops must be at least 2.
//
// An error is returned if the tolerance cannot be met within maxStops, or
// if the tolerance is invalid, the domain is degenerate or non-finite, or
// the gradient produces non-finite color values.
func (g Gradient) CssStopsApprox(tolerance float64, maxStops uint) (string, error) {
	min, max := g.Min, g.Max
	if math.IsNaN(tolerance) || math.IsInf(tolerance, 0) || tolerance < 0 {
		return "", fmt.Errorf("CssStopsApprox: tolerance must be a finite non-negative number, got %v", tolerance)
	}
	if maxStops < 2 {
		return "", fmt.Errorf("CssStopsApprox: maxStops must be at least 2, got %d", maxStops)
	}
	if !isFinite(min) || !isFinite(max) {
		return "", fmt.Errorf("CssStopsApprox: non-finite gradient domain: %v, %v", min, max)
	}
	if min >= max {
		return "", fmt.Errorf("CssStopsApprox: degenerate gradient domain: %v, %v", min, max)
	}

	// Fixed sampling grid, so output is deterministic.
	grid := make([]Color, cssGridRes+1)
	for i := range grid {
		u := float64(i) / cssGridRes
		col := g.At(min + u*(max-min))
		if !colorIsFinite(col) {
			return "", fmt.Errorf("CssStopsApprox: gradient contains non-finite color values")
		}
		grid[i] = col
	}

	budget := 2
	stops, hardEdges, err := cssSubdivide(grid, 0, cssGridRes, tolerance, maxStops, &budget)
	if err != nil {
		return "", err
	}

	css := formatCssStops(stops)

	// Verify the round trip on the same fixed grid. The rebuilt gradient
	// lives on the normalized [0, 1] domain, hence Core.At(u).
	rebuilt, err := NewGradient().Css(css).Build()
	if err != nil {
		return "", fmt.Errorf("CssStopsApprox: internal error, generated invalid CSS stops: %w", err)
	}

	hard := make(map[int]bool, len(hardEdges))
	for _, k := range hardEdges {
		// The in-between value at a hard-edge double stop is undefined.
		hard[k] = true
	}
	const slack = 1e-12
	for i := 0; i <= cssGridRes; i++ {
		if hard[i] {
			continue
		}
		u := float64(i) / cssGridRes
		if d := colorDistance(grid[i], rebuilt.Core.At(u)); d > tolerance+slack {
			return "", fmt.Errorf("CssStopsApprox: can not approximate gradient within tolerance %g using at most %d stops", tolerance, maxStops)
		}
	}

	return css, nil
}

type cssStopSample struct {
	u float64
	c Color
}

// cssSubdivide recursively fits grid[lo..hi] with piecewise-linear RGB
// stops and returns the ordered stop list. A discontinuity between grid
// cells k-1 and k is reported through hardEdges as grid index k-1 and
// emits two same-position stops carrying the one-sided limit colors.
// budget counts committed stops, starting at 2 for the two endpoints.
func cssSubdivide(grid []Color, lo, hi int, tolerance float64, maxStops uint, budget *int) ([]cssStopSample, []int, error) {
	c0 := grid[lo]
	c1 := grid[hi]

	kMax := -1
	maxErr := 0.0
	for k := lo + 1; k < hi; k++ {
		u := float64(k-lo) / float64(hi-lo)
		interp := blendRgb(c0, c1, u)
		if d := colorDistance(grid[k], interp); d > maxErr {
			maxErr = d
			kMax = k
		}
	}

	if maxErr <= tolerance {
		return []cssStopSample{
			{u: float64(lo) / float64(len(grid)-1), c: c0},
			{u: float64(hi) / float64(len(grid)-1), c: c1},
		}, nil, nil
	}

	// Hard-edge detection. A genuine discontinuity has a jump between two
	// neighbouring samples that dominates the steps immediately on both
	// sides of it: a steep but smooth curve changes at a comparable rate in
	// the neighbouring cells, whereas a hard edge does not. Adding a single
	// interior stop can never converge to a jump, so it is emitted as two
	// stops sharing one position.
	jump := colorDistance(grid[kMax-1], grid[kMax])
	leftStep := colorDistance(grid[kMax-2], grid[kMax-1])
	rightStep := colorDistance(grid[kMax], grid[kMax+1])
	isHardEdge := jump > 1e-12 &&
		jump >= 1.5*maxErr &&
		jump >= 2*leftStep &&
		jump >= 2*rightStep
	if isHardEdge {
		if *budget+2 > int(maxStops) {
			return nil, nil, fmt.Errorf("CssStopsApprox: can not approximate gradient within tolerance %g using at most %d stops", tolerance, maxStops)
		}
		*budget += 2

		// The double stop is aligned with grid point kMax-1, the last sample
		// before the jump. The rebuilt binary search lands on the left
		// segment for an equal-position pair, so the right color takes over
		// from the next grid point kMax onward; the point at the double stop
		// itself is skipped during verification as its in-between value is
		// undefined.
		edgeU := float64(kMax-1) / float64(len(grid)-1)
		stops := make([]cssStopSample, 0, 8)
		hard := []int{kMax - 1}

		if kMax-1 > lo {
			left, hardL, err := cssSubdivide(grid, lo, kMax-1, tolerance, maxStops, budget)
			if err != nil {
				return nil, nil, err
			}
			// Drop the shared endpoint, it is re-emitted as the edge stop.
			stops = append(stops, left[:len(left)-1]...)
			hard = append(hard, hardL...)
		} else {
			stops = append(stops, cssStopSample{
				u: float64(lo) / float64(len(grid)-1),
				c: grid[lo],
			})
		}

		stops = append(stops,
			cssStopSample{u: edgeU, c: grid[kMax-1]},
			cssStopSample{u: edgeU, c: grid[kMax]},
		)

		if kMax < hi {
			right, hardR, err := cssSubdivide(grid, kMax, hi, tolerance, maxStops, budget)
			if err != nil {
				return nil, nil, err
			}
			stops = append(stops, right...)
			hard = append(hard, hardR...)
		} else {
			stops = append(stops, cssStopSample{
				u: float64(hi) / float64(len(grid)-1),
				c: grid[hi],
			})
		}
		return stops, hard, nil
	}

	if *budget+1 > int(maxStops) {
		return nil, nil, fmt.Errorf("CssStopsApprox: can not approximate gradient within tolerance %g using at most %d stops", tolerance, maxStops)
	}
	*budget++

	left, hardL, err := cssSubdivide(grid, lo, kMax, tolerance, maxStops, budget)
	if err != nil {
		return nil, nil, err
	}
	right, hardR, err := cssSubdivide(grid, kMax, hi, tolerance, maxStops, budget)
	if err != nil {
		return nil, nil, err
	}
	return append(left, right[1:]...), append(hardL, hardR...), nil
}

// colorDistance returns the maximum absolute difference across the RGBA
// channels, so every channel is bounded by the same tolerance.
func colorDistance(a, b Color) float64 {
	return math.Max(
		math.Max(math.Abs(a.R-b.R), math.Abs(a.G-b.G)),
		math.Max(math.Abs(a.B-b.B), math.Abs(a.A-b.A)),
	)
}

func formatCssStops(stops []cssStopSample) string {
	var b []byte
	for i, st := range stops {
		if i > 0 {
			b = append(b, ',', ' ')
		}
		b = append(b, formatCssColor(st.c)...)
		b = append(b, ' ')
		b = append(b, formatPercent(st.u)...)
	}
	return string(b)
}

func formatCssColor(c Color) string {
	// csscolorparser interprets bare rgb() channel numbers as 0..255, so
	// channels are emitted as percentages to carry the 0..1 values without
	// quantization; alpha is a bare 0..1 number.
	s := "rgb(" +
		formatPercent(c.R) + " " +
		formatPercent(c.G) + " " +
		formatPercent(c.B)
	if c.A != 1 {
		s += " / " + formatCssFloat(c.A)
	}
	return s + ")"
}

func formatCssFloat(f float64) string {
	return strconv.FormatFloat(f, 'g', -1, 64)
}

func formatPercent(f float64) string {
	return strconv.FormatFloat(clamp01(f)*100, 'g', -1, 64) + "%"
}

func isFinite(f float64) bool {
	return !math.IsNaN(f) && !math.IsInf(f, 0)
}

func colorIsFinite(c Color) bool {
	return isFinite(c.R) && isFinite(c.G) && isFinite(c.B) && isFinite(c.A)
}
