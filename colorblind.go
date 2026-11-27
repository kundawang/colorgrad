package colorgrad

import (
	"fmt"
	"math"
)

// ColorblindConflict describes a pair of adjacent gradient samples that are
// hard to distinguish after color vision deficiency simulation.
type ColorblindConflict struct {
	// Index is the index of the first color in the sampled pair.
	// The second color is at index Index+1.
	Index uint
	// Position1 and Position2 are the gradient positions of the two colors.
	Position1 float64
	Position2 float64
	// Color1 and Color2 are the original sampled colors, alpha composited
	// over the specified background.
	Color1 Color
	Color2 Color
	// Distance is the perceptual distance (Euclidean distance in OKLab)
	// between the two colors after deuteranopia simulation.
	Distance float64
}

// ColorblindCheck samples count colors evenly across the gradient, simulates
// deuteranopia (red-green color vision deficiency) and reports every pair of
// adjacent samples whose perceptual distance is below threshold.
//
// The simulation uses the Brettel-Vienot-Mollon deuteranopia model.
// The perceptual distance is the Euclidean distance between the simulated
// colors in OKLab color space.
//
// Semi-transparent colors are alpha composited over bg before simulation.
// The original gradient is never modified.
//
// It returns an error when count is smaller than 2, when threshold is
// negative or not finite, when bg is not finite, or when any sampled color
// is not finite (NaN or +/-Inf).
func (g Gradient) ColorblindCheck(count uint, threshold float64, bg Color) ([]ColorblindConflict, error) {
	if count < 2 {
		return nil, fmt.Errorf("colorblind check requires at least 2 colors, got %d", count)
	}
	if !isFinite(threshold) || threshold < 0 {
		return nil, fmt.Errorf("invalid threshold: %v", threshold)
	}
	if !isFiniteColor(bg) {
		return nil, fmt.Errorf("invalid background color: %v", bg)
	}

	positions := linspace(g.dmin, g.dmax, count)
	simulated := make([][3]float64, count)
	composited := make([]Color, count)

	for i, pos := range positions {
		raw := g.At(pos)
		if !isFiniteColor(raw) {
			return nil, fmt.Errorf("non-finite color sampled at position %v", pos)
		}
		col := raw.Clamp()
		col = compositeOver(col, bg)
		composited[i] = col
		simulated[i] = simulateDeuteranopia(col.R, col.G, col.B)
	}

	conflicts := make([]ColorblindConflict, 0)
	for i := 0; i < len(simulated)-1; i++ {
		distance := oklabDistance(simulated[i], simulated[i+1])
		if distance < threshold {
			conflicts = append(conflicts, ColorblindConflict{
				Index:     uint(i),
				Position1: positions[i],
				Position2: positions[i+1],
				Color1:    composited[i],
				Color2:    composited[i+1],
				Distance:  distance,
			})
		}
	}
	return conflicts, nil
}

// compositeOver returns foreground alpha composited over background
// (source-over, W3C compositing rules). The returned color is fully opaque.
func compositeOver(fg, bg Color) Color {
	a := clamp01(fg.A)
	return Color{
		R: fg.R*a + bg.R*(1-a),
		G: fg.G*a + bg.G*(1-a),
		B: fg.B*a + bg.B*(1-a),
		A: 1,
	}
}

// oklabDistance returns the Euclidean distance between two sRGB colors
// in OKLab color space.
func oklabDistance(rgb1, rgb2 [3]float64) float64 {
	lab1 := col2oklab(Rgb(rgb1[0], rgb1[1], rgb1[2], 1))
	lab2 := col2oklab(Rgb(rgb2[0], rgb2[1], rgb2[2], 1))
	da := lab1[0] - lab2[0]
	db := lab1[1] - lab2[1]
	dc := lab1[2] - lab2[2]
	return math.Sqrt(da*da + db*db + dc*dc)
}

func isFinite(x float64) bool {
	return !math.IsNaN(x) && !math.IsInf(x, 0)
}

func isFiniteColor(c Color) bool {
	return isFinite(c.R) && isFinite(c.G) && isFinite(c.B) && isFinite(c.A)
}

// Deuteranopia simulation based on the Brettel, Vienot and Mollon (1997/1999)
// model, using the sRGB-to-LMS, dichromatic projection and LMS-to-sRGB
// matrices as published in Wickline's public-domain "Daltonize"
// (http://www.daltonize.org/2010/05/its-open-source-time.html).
//
// The matrices operate directly on gamma-encoded sRGB values, matching the
// widely used reference implementations (e.g. the "color-blind" package).
// The full LMS round trip is pre-multiplied below into a single 3x3 matrix.

var (
	// sRGB (gamma-encoded) to LMS.
	rgb2lms = [3][3]float64{
		{17.8824, 43.5161, 4.11935},
		{3.45565, 27.1554, 3.86714},
		{0.0299566, 0.184309, 1.46709},
	}
	// Deuteranopia projection in LMS space: the M-cone response is
	// reconstructed from the L and S responses along the dichromatic
	// confusion line.
	deutanProjection = [3][3]float64{
		{1, 0, 0},
		{0.494207, 0, 1.24827},
		{0, 0, 1},
	}
	// LMS to sRGB (gamma-encoded).
	lms2rgb = [3][3]float64{
		{0.0809444479, -0.130504409, 0.116721066},
		{-0.0102485335, 0.0540193266, -0.113614708},
		{-0.000365296938, -0.00412161469, 0.693511405},
	}
	// deutanMatrix = lms2rgb * deutanProjection * rgb2lms,
	// applied directly to gamma-encoded sRGB.
	deutanMatrix = matMul3(lms2rgb, matMul3(deutanProjection, rgb2lms))
)

// simulateDeuteranopia transforms a single sRGB color (channels in 0..1)
// into how it is perceived under deuteranopia. Output channels are clamped
// to the sRGB gamut 0..1.
func simulateDeuteranopia(r, g, b float64) [3]float64 {
	in := [3]float64{r, g, b}
	out := [3]float64{}
	for i := 0; i < 3; i++ {
		v := deutanMatrix[i][0]*in[0] + deutanMatrix[i][1]*in[1] + deutanMatrix[i][2]*in[2]
		out[i] = clamp01(v)
	}
	return out
}

// matMul3 returns the matrix product a*b for 3x3 matrices.
func matMul3(a, b [3][3]float64) [3][3]float64 {
	var p [3][3]float64
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			for k := 0; k < 3; k++ {
				p[i][j] += a[i][k] * b[k][j]
			}
		}
	}
	return p
}
