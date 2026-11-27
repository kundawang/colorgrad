package colorgrad

import (
	"fmt"
	"math"
)

// GridGradient is a two-dimensional color gradient defined by a rectangular
// grid of control colors. The grid has len(xs) columns and len(ys) rows,
// colors[j][i] is the color at position (xs[i], ys[j]), and the color at any
// point inside a cell is computed by bilinear interpolation in the selected
// blend mode.
type GridGradient struct {
	// Working-space representation of the control colors, colors[j][i].
	colors [][][4]float64
	// Original control colors, returned as-is at grid knots so that the four
	// corners (and every other knot) are exact in every blend mode.
	raw  [][]Color
	xs   []float64
	ys   []float64
	mode BlendMode
}

// NewGridGradient creates a two-dimensional gradient from ordered stop
// positions xs and ys and a rectangular color grid. colors must have len(ys)
// rows, each of len(xs) columns, with colors[j][i] located at (xs[i], ys[j]).
//
// The stop positions must be finite and strictly increasing, so duplicate
// coordinates are rejected. Grid points outside the domain are clamped to the
// borders. RGB, LinearRGB, Lab and Oklab blend modes are supported, including
// alpha interpolation.
func NewGridGradient(xs, ys []float64, colors [][]Color, mode BlendMode) (GridGradient, error) {
	if len(xs) < 2 {
		return GridGradient{}, fmt.Errorf("xs must contain at least 2 positions")
	}
	if len(ys) < 2 {
		return GridGradient{}, fmt.Errorf("ys must contain at least 2 positions")
	}
	for _, x := range xs {
		if !isFinite(x) {
			return GridGradient{}, fmt.Errorf("xs contains non-finite position")
		}
	}
	for _, y := range ys {
		if !isFinite(y) {
			return GridGradient{}, fmt.Errorf("ys contains non-finite position")
		}
	}
	for i := 1; i < len(xs); i++ {
		if xs[i] <= xs[i-1] {
			return GridGradient{}, fmt.Errorf("xs positions must be strictly increasing")
		}
	}
	for i := 1; i < len(ys); i++ {
		if ys[i] <= ys[i-1] {
			return GridGradient{}, fmt.Errorf("ys positions must be strictly increasing")
		}
	}
	if len(colors) != len(ys) {
		return GridGradient{}, fmt.Errorf("color grid has %d rows, expected %d", len(colors), len(ys))
	}
	for j, row := range colors {
		if len(row) != len(xs) {
			return GridGradient{}, fmt.Errorf("color grid row %d has %d colors, expected %d", j, len(row), len(xs))
		}
		for _, col := range row {
			if !isFinite(col.R) || !isFinite(col.G) || !isFinite(col.B) || !isFinite(col.A) {
				return GridGradient{}, fmt.Errorf("color grid contains non-finite color")
			}
		}
	}
	switch mode {
	case BlendRgb, BlendLinearRgb, BlendLab, BlendOklab:
	default:
		return GridGradient{}, fmt.Errorf("invalid blend mode: %d", mode)
	}

	// Defensive copies, so later mutation of the caller's slices cannot
	// change the gradient.
	cxs := make([]float64, len(xs))
	copy(cxs, xs)
	cys := make([]float64, len(ys))
	copy(cys, ys)

	grid := make([][][4]float64, len(ys))
	raw := make([][]Color, len(ys))
	for j, row := range colors {
		grid[j] = convertColors(row, mode)
		raw[j] = make([]Color, len(row))
		copy(raw[j], row)
	}

	return GridGradient{
		colors: grid,
		raw:    raw,
		xs:     cxs,
		ys:     cys,
		mode:   mode,
	}, nil
}

// At returns the interpolated color at position (x, y). Positions outside the
// gradient domain are clamped to the borders. A non-finite (NaN) coordinate
// returns opaque black.
func (g GridGradient) At(x, y float64) Color {
	if g.colors == nil {
		// Zero value or a constructor error.
		return Color{}
	}
	if math.IsNaN(x) || math.IsNaN(y) {
		return Color{A: 1}
	}

	ix, tx := locateCell(g.xs, x)
	iy, ty := locateCell(g.ys, y)

	// At any grid knot return the stored color as-is, so every control point,
	// including all four corners, is exact regardless of the blend mode.
	if tx == 0 && ty == 0 {
		return g.raw[iy][ix]
	}
	if tx == 1 && ty == 0 {
		return g.raw[iy][ix+1]
	}
	if tx == 0 && ty == 1 {
		return g.raw[iy+1][ix]
	}
	if tx == 1 && ty == 1 {
		return g.raw[iy+1][ix+1]
	}

	c00 := g.colors[iy][ix]
	c10 := g.colors[iy][ix+1]
	c01 := g.colors[iy+1][ix]
	c11 := g.colors[iy+1][ix+1]

	// Bilinear interpolation: along x first, then along y.
	ct := lerp4(c00, c10, tx)
	cb := lerp4(c01, c11, tx)
	return g.toColor(lerp4(ct, cb, ty))
}

// Domain returns the stop position ranges: (xmin, xmax, ymin, ymax).
func (g GridGradient) Domain() (xmin, xmax, ymin, ymax float64) {
	if g.colors == nil {
		return 0, 0, 0, 0
	}
	return g.xs[0], g.xs[len(g.xs)-1], g.ys[0], g.ys[len(g.ys)-1]
}

func (g GridGradient) toColor(c [4]float64) Color {
	switch g.mode {
	case BlendRgb:
		return Color{R: c[0], G: c[1], B: c[2], A: c[3]}
	case BlendLinearRgb:
		return LinearRgb(c[0], c[1], c[2], c[3])
	case BlendLab:
		return Lab(c[0], c[1], c[2], c[3]).Clamp()
	case BlendOklab:
		return Oklab(c[0], c[1], c[2], c[3]).Clamp()
	}
	return Color{}
}

// locateCell returns the index i of the cell containing t (stops[i] <= t <=
// stops[i+1]) and the normalized position within that cell. Values outside the
// stop range are clamped to the first or last cell.
func locateCell(stops []float64, t float64) (int, float64) {
	n := len(stops)
	if t <= stops[0] {
		return 0, 0
	}
	if t >= stops[n-1] {
		return n - 2, 1
	}

	low := 0
	high := n
	for low < high {
		mid := (low + high) / 2
		if stops[mid] < t {
			low = mid + 1
		} else {
			high = mid
		}
	}
	i := low - 1
	return i, (t - stops[i]) / (stops[i+1] - stops[i])
}

func lerp4(a, b [4]float64, t float64) [4]float64 {
	if t == 0 {
		return a
	}
	if t == 1 {
		return b
	}
	return [4]float64{
		a[0] + t*(b[0]-a[0]),
		a[1] + t*(b[1]-a[1]),
		a[2] + t*(b[2]-a[2]),
		a[3] + t*(b[3]-a[3]),
	}
}

func isFinite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}
