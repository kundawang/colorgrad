package colorgrad

import (
	"fmt"
	"math"
)

// Default precision for interval average color integration.
const defaultAverageEpsilon = 1e-8

// Default maximum number of adaptive subdivisions per interval piece.
const defaultAverageMaxDepth = 16

const averageMaxDepthLimit = 32

// AverageOption customizes the precision of [Gradient.AverageColor].
type AverageOption func(*averageOptions)

type averageOptions struct {
	epsilon  float64
	maxDepth int
}

// AverageEpsilon sets the per-channel absolute integration tolerance.
// Non-finite or non-positive values fall back to the default (1e-8).
func AverageEpsilon(eps float64) AverageOption {
	return func(o *averageOptions) {
		o.epsilon = eps
	}
}

// AverageMaxDepth sets the maximum number of adaptive subdivisions applied
// to every piece of the queried interval. Non-positive values fall back to
// the default (16); values above 32 are capped.
func AverageMaxDepth(depth int) AverageOption {
	return func(o *averageOptions) {
		o.maxDepth = depth
	}
}

// stopPositions is implemented by gradient cores whose color function has
// internal breakpoints (the stop positions). The interval is split at these
// points so that narrow color bands and hard edges cannot be missed.
type stopPositions interface {
	stopPositions() []float64
}

// AverageColor returns the coverage-weighted average color over the
// parameter interval [t0, t1].
//
// The average is computed in linear-light (linear sRGB) with premultiplied
// alpha, so fully transparent colors never bleed dark fringes into the
// result: colors are weighted by their alpha while accumulating, and the RGB
// channels are only unpremultiplied by the average alpha at the end.
//
// The precision of the numerical integration can be bounded through
// [AverageEpsilon] and [AverageMaxDepth].
//
// If t0 == t1 the color at that position is returned. Non-finite (NaN or
// Inf) interval endpoints result in an error. A reversed interval (t0 > t1)
// is swapped. Endpoints outside the gradient domain are clamped, matching
// the endpoint extension behavior of [Gradient.At]. The returned color is
// always a valid RGBA color with every channel in [0, 1].
func (g Gradient) AverageColor(t0, t1 float64, opts ...AverageOption) (Color, error) {
	o := averageOptions{
		epsilon:  defaultAverageEpsilon,
		maxDepth: defaultAverageMaxDepth,
	}
	for _, opt := range opts {
		opt(&o)
	}
	if math.IsNaN(o.epsilon) || math.IsInf(o.epsilon, 0) || o.epsilon <= 0 {
		o.epsilon = defaultAverageEpsilon
	}
	if o.maxDepth <= 0 {
		o.maxDepth = defaultAverageMaxDepth
	} else if o.maxDepth > averageMaxDepthLimit {
		o.maxDepth = averageMaxDepthLimit
	}

	if math.IsNaN(t0) || math.IsInf(t0, 0) || math.IsNaN(t1) || math.IsInf(t1, 0) {
		return Color{}, fmt.Errorf("AverageColor: non-finite interval endpoints: %v, %v", t0, t1)
	}

	if t0 > t1 {
		t0, t1 = t1, t0
	}

	// Clamp to the gradient domain; outside the domain At() is constant.
	lo := math.Max(t0, g.Min)
	hi := math.Min(t1, g.Max)

	if hi <= lo {
		// Zero-length (or empty after clamping) interval: point query.
		return g.At(math.Min(math.Max(t0, g.Min), g.Max)).Clamp(), nil
	}

	// Break the interval at every internal stop position. Each resulting
	// piece is smooth (or constant), so adaptive integration stays accurate
	// even for arbitrarily narrow bands and hard edges.
	bounds := make([]float64, 0, 8)
	bounds = append(bounds, lo)
	if sp, ok := g.Core.(stopPositions); ok {
		for _, p := range sp.stopPositions() {
			if p > lo && p < hi {
				bounds = append(bounds, p)
			}
		}
	}
	bounds = append(bounds, hi)

	sample := g.premultipliedLinearSample
	var sum [4]float64
	totalWidth := 0.0

	for i := 0; i+1 < len(bounds); i++ {
		a := bounds[i]
		b := bounds[i+1]
		if b <= a {
			continue
		}
		width := b - a
		totalWidth += width
		eps := o.epsilon * width / (hi - lo)
		// At a shared stop, At() follows the left-side convention. A
		// discontinuity point has zero measure, but the quadrature samples
		// it directly, so evaluate the right-hand limit there instead.
		pieceSample := func(x float64) vec4 {
			if x == a {
				x = math.Nextafter(a, b)
			}
			return sample(x)
		}
		intg := adaptiveSimpson(pieceSample, a, b, eps, o.maxDepth)
		for k := 0; k < 4; k++ {
			sum[k] += intg[k]
		}
	}

	// Average premultiplied linear-light channels over the interval.
	avgAlpha := sum[3] / totalWidth
	if avgAlpha <= 0 {
		// Nothing visible anywhere: canonical fully transparent black.
		return Color{}, nil
	}

	col := LinearRgb(
		sum[0]/totalWidth/avgAlpha,
		sum[1]/totalWidth/avgAlpha,
		sum[2]/totalWidth/avgAlpha,
		avgAlpha,
	).Clamp()

	if !isFiniteColor(col) {
		return Color{}, fmt.Errorf("AverageColor: non-finite result for interval [%v, %v]", t0, t1)
	}
	return col, nil
}

func isFiniteColor(c Color) bool {
	for _, v := range [4]float64{c.R, c.G, c.B, c.A} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}

// premultipliedLinearSample evaluates the gradient at t and returns the
// color as premultiplied linear-light (linear sRGB) channels:
// {R*a, G*a, B*a, a}. Alpha is clamped to [0, 1].
func (g Gradient) premultipliedLinearSample(t float64) [4]float64 {
	col := g.Core.At(t)
	a := clamp01(col.A)
	lr := toLinear(col.R)
	lg := toLinear(col.G)
	lb := toLinear(col.B)
	return [4]float64{lr * a, lg * a, lb * a, a}
}

// --- Adaptive Simpson integration (vectorized over the four channels) ---

type vec4 = [4]float64

func vecAdd(a, b vec4) vec4 {
	return vec4{a[0] + b[0], a[1] + b[1], a[2] + b[2], a[3] + b[3]}
}

func vecSub(a, b vec4) vec4 {
	return vec4{a[0] - b[0], a[1] - b[1], a[2] - b[2], a[3] - b[3]}
}

func vecScale(a vec4, s float64) vec4 {
	return vec4{a[0] * s, a[1] * s, a[2] * s, a[3] * s}
}

func vecWithinEps(a vec4, eps float64) bool {
	for i := 0; i < 4; i++ {
		if math.Abs(a[i]) > eps {
			return false
		}
	}
	return true
}

func simpson(f func(float64) vec4, a, b float64, fa, fm, fb vec4) vec4 {
	return vecScale(vecAdd(vecAdd(fa, vecScale(fm, 4)), fb), (b-a)/6)
}

// adaptiveSimpson integrates f over [a, b] until every channel is within
// eps of the refined estimate or the subdivision depth is exhausted.
func adaptiveSimpson(f func(float64) vec4, a, b, eps float64, maxDepth int) vec4 {
	m := (a + b) / 2
	fa := f(a)
	fm := f(m)
	fb := f(b)
	whole := simpson(f, a, b, fa, fm, fb)
	return adaptiveSimpsonRec(f, a, b, eps, fa, fm, fb, whole, maxDepth)
}

func adaptiveSimpsonRec(f func(float64) vec4, a, b, eps float64, fa, fm, fb, whole vec4, depth int) vec4 {
	m := (a + b) / 2
	lm := (a + m) / 2
	rm := (m + b) / 2
	flm := f(lm)
	frm := f(rm)
	left := simpson(f, a, m, fa, flm, fm)
	right := simpson(f, m, b, fm, frm, fb)
	refined := vecAdd(left, right)
	diff := vecSub(refined, whole)

	if depth <= 0 || vecWithinEps(diff, 15*eps) {
		// Richardson correction; the remaining error is bounded by eps.
		return vecAdd(refined, vecScale(diff, 1.0/15.0))
	}

	halfEps := eps / 2
	return vecAdd(
		adaptiveSimpsonRec(f, a, m, halfEps, fa, flm, fm, left, depth-1),
		adaptiveSimpsonRec(f, m, b, halfEps, fm, frm, fb, right, depth-1),
	)
}
