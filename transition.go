package colorgrad

import (
	"fmt"
	"math"
)

// LerpGradient returns a stop-aware transition between two linear gradients.
//
// Unlike blending the sampled colors of two gradients (which produces two
// half-transparent peaks when a stop moves), LerpGradient interpolates the
// stops themselves: each stop position and each stop color of a moves
// synchronously towards the corresponding stop of b, so a peak at 20%
// travels to 80% instead of cross-fading.
//
// Semantics:
//   - Both gradients must be linear gradients (built with InterpolationLinear),
//     have the same number of stops and use the same BlendMode.
//   - Stop positions are interpolated pairwise, so non-uniform domains are
//     supported: the resulting domain is the weighted mix of both domains.
//   - Stop colors are interpolated in the gradient's blend mode space
//     (RGB, linear-RGB, Lab or Oklab), alpha included, so transparent stops
//     fade in/out smoothly.
//   - Duplicated stop positions (hard edges) are preserved: if both inputs
//     have a hard edge at the i-th stop pair, the result has a hard edge at
//     the interpolated position.
//   - The weight t must be in [0, 1]; t == 0 yields exactly a's stops and
//     t == 1 yields exactly b's stops.
//
// The inputs are not modified. An error is returned if the stop counts
// differ, the blend modes differ, either gradient is not linear, a stop
// sequence is not sorted, or the weight is outside [0, 1] or NaN.
func LerpGradient(a, b Gradient, t float64) (Gradient, error) {
	fail := func(err error) (Gradient, error) {
		return Gradient{
			Core: zeroGradient{},
			Min:  0,
			Max:  1,
		}, err
	}

	if math.IsNaN(t) || t < 0 || t > 1 {
		return fail(fmt.Errorf("colorgrad: invalid transition weight %v (must be in range [0, 1])", t))
	}

	ga, ok := a.Core.(linearGradient)
	if !ok {
		return fail(fmt.Errorf("colorgrad: gradient transition requires linear gradients"))
	}
	gb, ok := b.Core.(linearGradient)
	if !ok {
		return fail(fmt.Errorf("colorgrad: gradient transition requires linear gradients"))
	}

	if ga.mode != gb.mode {
		return fail(fmt.Errorf("colorgrad: gradient transition requires identical blend modes (%s != %s)", ga.mode, gb.mode))
	}

	if len(ga.positions) != len(gb.positions) {
		return fail(fmt.Errorf("colorgrad: gradient transition requires equal stop counts (%d != %d)", len(ga.positions), len(gb.positions)))
	}

	n := len(ga.positions)
	for i := 1; i < n; i++ {
		if ga.positions[i] < ga.positions[i-1] || gb.positions[i] < gb.positions[i-1] {
			return fail(fmt.Errorf("colorgrad: gradient transition requires sorted stop positions"))
		}
	}

	positions := make([]float64, n)
	colors := make([][4]float64, n)

	for i := 0; i < n; i++ {
		positions[i] = ga.positions[i] + t*(gb.positions[i]-ga.positions[i])
		x, y, z, w := linearInterpolate(ga.colors[i], gb.colors[i], t)
		colors[i] = [4]float64{x, y, z, w}
	}

	grad := linearGradient{
		colors:    colors,
		positions: positions,
		min:       positions[0],
		max:       positions[n-1],
		mode:      ga.mode,
		first:     blendRgb(ga.first, gb.first, t),
		last:      blendRgb(ga.last, gb.last, t),
	}

	return Gradient{
		Core: grad,
		Min:  positions[0],
		Max:  positions[n-1],
	}, nil
}
