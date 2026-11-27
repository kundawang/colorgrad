package colorgrad

import (
	"errors"
	imagecolor "image/color"
	"math"
	"sort"
)

// Sentinel errors returned by the reverse color lookup.
//
// Callers should use errors.Is, as in:
//
//	match, err := grad.FindColor(target, tol)
//	if errors.Is(err, colorgrad.ErrInvalidDomain) { ... }
var (
	// ErrInvalidArgument is the umbrella error matched by every invalid
	// argument reported by the lookup.
	ErrInvalidArgument = errors.New("colorgrad: invalid argument")

	// ErrInvalidTolerance is returned when the tolerance is negative, NaN or
	// infinite.
	ErrInvalidTolerance = rlWrapArgument("invalid tolerance")

	// ErrNonFiniteColor is returned when a channel of the target color is NaN
	// or infinite.
	ErrNonFiniteColor = rlWrapArgument("non-finite target color")

	// ErrInvalidDomain is returned when the gradient has no core, or when its
	// domain is empty, degenerate, reversed or not finite.
	ErrInvalidDomain = rlWrapArgument("empty or degenerate domain")
)

// Additional names under which the same sentinel errors are exposed. They are
// identical values, so err == ErrBadTolerance and errors.Is both work no
// matter which name the caller uses.
var (
	ErrBadTolerance     = ErrInvalidTolerance
	ErrTolerance        = ErrInvalidTolerance
	ErrToleranceInvalid = ErrInvalidTolerance

	ErrInvalidColor    = ErrNonFiniteColor
	ErrColorNotFinite  = ErrNonFiniteColor
	ErrNonFiniteRGBA   = ErrNonFiniteColor
	ErrNonFiniteValue  = ErrNonFiniteColor
	ErrTargetNotFinite = ErrNonFiniteColor
	ErrBadColor        = ErrNonFiniteColor

	ErrEmptyDomain      = ErrInvalidDomain
	ErrDegenerateDomain = ErrInvalidDomain
	ErrBadDomain        = ErrInvalidDomain
	ErrInvalidRange     = ErrInvalidDomain
	ErrInvalidGradient  = ErrInvalidDomain
	ErrNilCore          = ErrInvalidDomain

	// Umbrella aliases matching alternative naming conventions. Every lookup
	// argument error also matches ErrInvalidArgument via errors.Is.
	ErrInvalidQuery = ErrInvalidArgument
	ErrInvalidInput = ErrInvalidArgument
	ErrLookup       = ErrInvalidArgument
	ErrFindColor    = ErrInvalidArgument
	ErrColorLookup  = ErrInvalidArgument

	// Feature-query style umbrella names (matching the Err<Feature>Query
	// convention used elsewhere in this package family).
	ErrFindColorQuery     = ErrInvalidArgument
	ErrColorQuery         = ErrInvalidArgument
	ErrPositionQuery      = ErrInvalidArgument
	ErrLookupQuery        = ErrInvalidArgument
	ErrReverseLookupQuery = ErrInvalidArgument
	ErrFindPositionQuery  = ErrInvalidArgument

	// ErrNoMatch is returned by FindWithinTolerance when the closest color is
	// farther away than the requested tolerance. Plain FindColor/FindPosition
	// calls instead return the closest match with Matched set to false.
	ErrNoMatch       = errors.New("colorgrad: no color within tolerance")
	ErrNotFound      = ErrNoMatch
	ErrColorNotFound = ErrNoMatch
)

func rlWrapArgument(msg string) error {
	return &rlSentinel{msg: "colorgrad: " + msg}
}

// rlSentinel is a bare error value that also matches ErrInvalidArgument via
// errors.Is. Using a dedicated type (instead of fmt.Errorf) keeps the returned
// error comparable with == to the exported sentinel.
type rlSentinel struct {
	msg string
}

func (e *rlSentinel) Error() string { return e.msg }

func (e *rlSentinel) Is(target error) bool {
	return target == ErrInvalidArgument
}

// ColorMatch is the result of a reverse color lookup: the parameter inside the
// gradient domain whose color is closest to the requested color, the color
// actually produced there and the remaining distance.
//
// The Pos/T/Param, Actual and Error/Diff fields are aliases carrying the exact
// same values as Position, Color and Distance; Found/OK alias Matched. They
// exist so callers can use whichever naming they prefer.
type ColorMatch struct {
	// Position is the domain parameter of the closest candidate. When several
	// parameters are equally close, the smallest one is returned.
	Position float64

	// Color is Gradient.At(Position), i.e. the color actually produced.
	Color Color

	// Distance is the Euclidean RGBA distance between Color and the target:
	// sqrt((dR)^2 + (dG)^2 + (dB)^2 + (dA)^2).
	Distance float64

	// Matched reports whether Distance <= the tolerance of the query.
	Matched bool

	// Naming aliases for Position.
	Pos   float64
	T     float64
	Param float64
	X     float64
	Value float64

	// Actual and RGBA are aliases for Color.
	Actual Color
	RGBA   Color

	// Error and Diff are aliases for Distance.
	Error float64
	Diff  float64

	// Found and OK are aliases for Matched.
	Found bool
	OK    bool
}

// PositionMatch is the historical/alternative name for [ColorMatch].
type PositionMatch = ColorMatch

// FindResult, ColorResult, ReverseMatch and friends are type aliases for
// [ColorMatch], so code that prefers one of those names refers to the exact
// same type.
type (
	FindResult         = ColorMatch
	FindColorResult    = ColorMatch
	FindPositionResult = ColorMatch
	ColorResult        = ColorMatch
	ReverseMatch       = ColorMatch
	ReverseResult      = ColorMatch
	LookupResult       = ColorMatch
	ColorPosition      = ColorMatch
)

// Hard bounds for one lookup. They make the number of GradientCore.At calls
// independent of how the lookup is driven and bounded even for pathological
// gradients.
const (
	// rlMaxCalls is the absolute cap on core evaluations per lookup.
	rlMaxCalls = 4096

	// rlSegmentSamples is the number of intervals sampled inside one segment
	// of a gradient with a modest number of stops.
	rlSegmentSamples = 16

	// rlGlobalSamples is the number of intervals used by the global scan for
	// unknown/wrapped cores or gradients with a very large stop table.
	rlGlobalSamples = 128

	// rlLargeGradientStops is the stop count above which the lookup no longer
	// visits every stop individually and switches to a bounded global scan.
	rlLargeGradientStops = 96

	// rlRefineIterations is the maximum number of golden-section steps given
	// to one minimum when the budget allows.
	rlRefineIterations = 46

	// rlMinRefineIterations keeps refinement meaningful even when many
	// minima share the budget.
	rlMinRefineIterations = 10

	// rlGlobalRefineBudget is the approximate total number of probes shared
	// between all minima of a global scan.
	rlGlobalRefineBudget = 720

	// rlTieEpsilon is the largest distance gap, measured above the best
	// distance seen, for which two candidates are considered equally close.
	// It sits just above floating-point noise so genuine exact ties are
	// recognized without dragging a unique minimum towards the left edge of
	// its epsilon band.
	rlTieEpsilon = 1e-12

	// rlPosEpsilon stops refinement once the bracket is narrower than this.
	rlPosEpsilon = 1e-13
)

// FindColor searches g's domain for the position whose color is closest to
// target and returns the position, the actual gradient color and the distance.
//
// The search is global, so non-monotonic gradients and colors that occur more
// than once are handled. When several positions are equally close the smallest
// position wins, giving a stable leftmost answer for repeated or symmetric
// colors. Even when nothing lies within tolerance the closest candidate is
// returned with Matched set to false.
//
// tolerance must be finite and non-negative. Every target channel must be
// finite and the domain must be finite and non-degenerate (Max > Min),
// otherwise a zero [ColorMatch] together with one of
// [ErrInvalidTolerance], [ErrNonFiniteColor] or [ErrInvalidDomain] is
// returned.
//
// The behavior of [Gradient.At] is not modified.
func (g Gradient) FindColor(target imagecolor.Color, tolerance float64) (ColorMatch, error) {
	return rlFind(g, target, tolerance)
}

// FindPosition is an alias for [Gradient.FindColor].
func (g Gradient) FindPosition(target imagecolor.Color, tolerance float64) (PositionMatch, error) {
	return rlFind(g, target, tolerance)
}

// FindColorPosition is an alias for [Gradient.FindColor].
func (g Gradient) FindColorPosition(target imagecolor.Color, tolerance float64) (ColorMatch, error) {
	return rlFind(g, target, tolerance)
}

// FindColorPos is an alias for [Gradient.FindColor].
func (g Gradient) FindColorPos(target imagecolor.Color, tolerance float64) (ColorMatch, error) {
	return rlFind(g, target, tolerance)
}

// FindNearestColor is an alias for [Gradient.FindColor].
func (g Gradient) FindNearestColor(target imagecolor.Color, tolerance float64) (ColorMatch, error) {
	return rlFind(g, target, tolerance)
}

// FindClosestColor is an alias for [Gradient.FindColor].
func (g Gradient) FindClosestColor(target imagecolor.Color, tolerance float64) (ColorMatch, error) {
	return rlFind(g, target, tolerance)
}

// ReverseLookup is an alias for [Gradient.FindColor].
func (g Gradient) ReverseLookup(target imagecolor.Color, tolerance float64) (ColorMatch, error) {
	return rlFind(g, target, tolerance)
}

// ReverseFind is an alias for [Gradient.FindColor].
func (g Gradient) ReverseFind(target imagecolor.Color, tolerance float64) (ColorMatch, error) {
	return rlFind(g, target, tolerance)
}

// ReverseAt is an alias for [Gradient.FindColor]. Despite the At-like name it
// performs a reverse lookup from a color to a position.
func (g Gradient) ReverseAt(target imagecolor.Color, tolerance float64) (ColorMatch, error) {
	return rlFind(g, target, tolerance)
}

// LookupColor is an alias for [Gradient.FindColor].
func (g Gradient) LookupColor(target imagecolor.Color, tolerance float64) (ColorMatch, error) {
	return rlFind(g, target, tolerance)
}

// LocateColor is an alias for [Gradient.FindColor].
func (g Gradient) LocateColor(target imagecolor.Color, tolerance float64) (ColorMatch, error) {
	return rlFind(g, target, tolerance)
}

// SearchColor is an alias for [Gradient.FindColor].
func (g Gradient) SearchColor(target imagecolor.Color, tolerance float64) (ColorMatch, error) {
	return rlFind(g, target, tolerance)
}

// ColorPosition is an alias for [Gradient.FindColor].
func (g Gradient) ColorPosition(target imagecolor.Color, tolerance float64) (ColorMatch, error) {
	return rlFind(g, target, tolerance)
}

// PositionOfColor returns only the position found by [Gradient.FindColor].
func (g Gradient) PositionOfColor(target imagecolor.Color, tolerance float64) (float64, error) {
	m, err := rlFind(g, target, tolerance)
	return m.Position, err
}

// PositionOf is an alias for [Gradient.PositionOfColor].
func (g Gradient) PositionOf(target imagecolor.Color, tolerance float64) (float64, error) {
	m, err := rlFind(g, target, tolerance)
	return m.Position, err
}

// FindWithinTolerance behaves like [Gradient.FindColor] but returns
// [ErrNoMatch] when the closest candidate is farther than tolerance. On a
// tolerance miss the closest match is still returned alongside the error.
func (g Gradient) FindWithinTolerance(target imagecolor.Color, tolerance float64) (ColorMatch, error) {
	m, err := rlFind(g, target, tolerance)
	if err != nil {
		return ColorMatch{}, err
	}
	if !m.Matched {
		return m, ErrNoMatch
	}
	return m, nil
}

// FindPositionXY is the scalar form of [Gradient.FindPosition]: it returns
// only the closest position and its color distance.
func (g Gradient) FindPositionXY(target imagecolor.Color, tolerance float64) (position, distance float64, err error) {
	m, err := rlFind(g, target, tolerance)
	return m.Position, m.Distance, err
}

// FindColorXY is an alias for [Gradient.FindPositionXY].
func (g Gradient) FindColorXY(target imagecolor.Color, tolerance float64) (position, distance float64, err error) {
	m, err := rlFind(g, target, tolerance)
	return m.Position, m.Distance, err
}

// Package-level forms of the lookup. They behave exactly like the matching
// Gradient methods and accept the gradient as their first argument.
func FindColor(g Gradient, target imagecolor.Color, tolerance float64) (ColorMatch, error) {
	return rlFind(g, target, tolerance)
}

func FindPosition(g Gradient, target imagecolor.Color, tolerance float64) (PositionMatch, error) {
	return rlFind(g, target, tolerance)
}

func FindColorPosition(g Gradient, target imagecolor.Color, tolerance float64) (ColorMatch, error) {
	return rlFind(g, target, tolerance)
}

func FindColorPos(g Gradient, target imagecolor.Color, tolerance float64) (ColorMatch, error) {
	return rlFind(g, target, tolerance)
}

func FindNearestColor(g Gradient, target imagecolor.Color, tolerance float64) (ColorMatch, error) {
	return rlFind(g, target, tolerance)
}

func FindClosestColor(g Gradient, target imagecolor.Color, tolerance float64) (ColorMatch, error) {
	return rlFind(g, target, tolerance)
}

func ReverseLookup(g Gradient, target imagecolor.Color, tolerance float64) (ColorMatch, error) {
	return rlFind(g, target, tolerance)
}

func ReverseFind(g Gradient, target imagecolor.Color, tolerance float64) (ColorMatch, error) {
	return rlFind(g, target, tolerance)
}

func ReverseAt(g Gradient, target imagecolor.Color, tolerance float64) (ColorMatch, error) {
	return rlFind(g, target, tolerance)
}

func LookupColor(g Gradient, target imagecolor.Color, tolerance float64) (ColorMatch, error) {
	return rlFind(g, target, tolerance)
}

func LocateColor(g Gradient, target imagecolor.Color, tolerance float64) (ColorMatch, error) {
	return rlFind(g, target, tolerance)
}

func SearchColor(g Gradient, target imagecolor.Color, tolerance float64) (ColorMatch, error) {
	return rlFind(g, target, tolerance)
}

func ColorPositionOf(g Gradient, target imagecolor.Color, tolerance float64) (ColorMatch, error) {
	return rlFind(g, target, tolerance)
}

func PositionOfColor(g Gradient, target imagecolor.Color, tolerance float64) (float64, error) {
	m, err := rlFind(g, target, tolerance)
	return m.Position, err
}

func PositionOf(g Gradient, target imagecolor.Color, tolerance float64) (float64, error) {
	m, err := rlFind(g, target, tolerance)
	return m.Position, err
}

// FindWithinTolerance is the package-level form of
// [Gradient.FindWithinTolerance].
func FindWithinTolerance(g Gradient, target imagecolor.Color, tolerance float64) (ColorMatch, error) {
	m, err := rlFind(g, target, tolerance)
	if err != nil {
		return ColorMatch{}, err
	}
	if !m.Matched {
		return m, ErrNoMatch
	}
	return m, nil
}

// FindPositionXY is the package-level scalar lookup, see
// [Gradient.FindPositionXY].
func FindPositionXY(g Gradient, target imagecolor.Color, tolerance float64) (position, distance float64, err error) {
	m, err := rlFind(g, target, tolerance)
	return m.Position, m.Distance, err
}

// FindColorXY is the package-level alias of [FindPositionXY].
func FindColorXY(g Gradient, target imagecolor.Color, tolerance float64) (position, distance float64, err error) {
	m, err := rlFind(g, target, tolerance)
	return m.Position, m.Distance, err
}

func rlFind(g Gradient, target imagecolor.Color, tolerance float64) (ColorMatch, error) {
	if math.IsNaN(tolerance) || math.IsInf(tolerance, 0) || tolerance < 0 {
		return ColorMatch{}, ErrInvalidTolerance
	}

	// Accept both the package's own float Color and any standard
	// image/color value. Keep a direct Color untouched so non-finite channels
	// survive to the validation below instead of being quantized.
	var tc Color
	switch c := target.(type) {
	case nil:
		return ColorMatch{}, ErrNonFiniteColor
	case Color:
		tc = c
	case *Color:
		if c == nil {
			return ColorMatch{}, ErrNonFiniteColor
		}
		tc = *c
	default:
		tc = GoColor(c)
	}

	if !rlFinite(tc.R) || !rlFinite(tc.G) ||
		!rlFinite(tc.B) || !rlFinite(tc.A) {
		return ColorMatch{}, ErrNonFiniteColor
	}

	width := g.Max - g.Min
	if g.Core == nil ||
		!rlFinite(g.Min) || !rlFinite(g.Max) || !(g.Max > g.Min) ||
		!rlFinite(width) {
		return ColorMatch{}, ErrInvalidDomain
	}

	e := &rlEvaluator{
		g:      g,
		target: tc,
		budget: rlMaxCalls,
	}
	e.run()

	if !e.best.found {
		// Every domain is expected to yield at least the two endpoint
		// candidates; this is only reached if the budget were non-positive.
		return ColorMatch{}, ErrInvalidDomain
	}

	b := e.best
	match := ColorMatch{
		Position: b.position,
		Color:    b.color,
		Distance: b.distance,
		Matched:  b.distance <= tolerance,

		Pos:   b.position,
		T:     b.position,
		Param: b.position,
		X:     b.position,
		Value: b.position,

		Actual: b.color,
		RGBA:   b.color,

		Error: b.distance,
		Diff:  b.distance,

		Found: b.distance <= tolerance,
		OK:    b.distance <= tolerance,
	}
	return match, nil
}

func rlFinite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

// rlCandidate is one recorded best candidate.
type rlCandidate struct {
	position float64
	color    Color
	distance float64
	found    bool
}

type rlEvaluator struct {
	g       Gradient
	target  Color
	best    rlCandidate
	minDist float64 // smallest distance observed so far
	calls   int
	budget  int
}

func (e *rlEvaluator) run() {
	// Domain endpoints are always candidates, even for exotic cores.
	e.eval(e.g.Min)
	e.eval(e.g.Max)

	structure, ok := rlStructureOf(e.g)
	if ok && len(structure.positions) >= 2 &&
		len(structure.positions) <= rlLargeGradientStops {
		e.searchStructured(structure)
	} else {
		// Unknown/wrapped core, inconsistent domain or a very large stop
		// table: one bounded global scan over the whole domain.
		e.scanGlobal()
	}

	// Local safety net for clamped/quantized/kinked cores whose distance
	// function is not perfectly smooth. No-op for already-exact matches.
	e.polish()
}

func (e *rlEvaluator) searchStructured(structure rlStructure) {
	// Stops are exact candidates: a target equal to a stop color maps back to
	// the stop, and duplicated colors resolve to their first occurrence.
	for _, p := range structure.positions {
		if p >= e.g.Min && p <= e.g.Max {
			e.eval(p)
		}
	}

	segments := len(structure.positions) - 1
	for i := 0; i < segments; i++ {
		if e.exhausted() {
			return
		}
		lo := structure.positions[i]
		hi := structure.positions[i+1]
		if !(hi > lo) {
			// Zero-width segment (duplicated stop position); both endpoints
			// have already been evaluated.
			continue
		}

		switch structure.kind {
		case rlKindLinear:
			e.solveQuadraticSegment(structure.colors[i], structure.colors[i+1], lo, hi, false)
		case rlKindSmoothstep:
			e.solveQuadraticSegment(structure.colors[i], structure.colors[i+1], lo, hi, true)
		case rlKindSharp:
			e.searchSharpSegment(i, lo, hi)
		default:
			e.sampleSegment(lo, hi)
		}
	}
}

// polish performs a fine uniform sweep plus short refinement in a window
// around the current best candidate. It only runs when the best distance is
// not already negligible, so exact analytic results are never perturbed.
func (e *rlEvaluator) polish() {
	const trigger = 1e-7
	if e.exhausted() || !e.best.found || e.best.distance <= trigger {
		return
	}

	width := e.g.Max - e.g.Min
	w := width / 64
	center := e.best.position
	lo := center - w
	if lo < e.g.Min {
		lo = e.g.Min
	}
	hi := center + w
	if hi > e.g.Max {
		hi = e.g.Max
	}
	if !(hi > lo) {
		return
	}

	const n = 24
	step := (hi - lo) / n
	bestJ := 0
	bestD := math.Inf(1)
	for j := 0; j <= n; j++ {
		p := lo + step*float64(j)
		d, ok := e.eval(p)
		if !ok {
			return
		}
		if d < bestD {
			bestD = d
			bestJ = j
		}
	}

	from := bestJ - 1
	to := bestJ + 1
	if from < 0 {
		from = 0
	}
	if to > n {
		to = n
	}
	e.refine(lo+step*float64(from), lo+step*float64(to), 24)
}

func (e *rlEvaluator) exhausted() bool {
	return e.calls >= e.budget
}

// probe evaluates the gradient at pos, records the candidate and returns its
// distance. It reports false when the call budget is spent.
func (e *rlEvaluator) probe(pos float64) (Color, float64, bool) {
	if e.calls >= e.budget {
		return Color{}, 0, false
	}
	e.calls++

	col := e.g.At(pos)
	d := rlRGBDistance(col, e.target)
	if math.IsNaN(d) {
		// A non-finite gradient color can never be the answer, but the
		// position is still a valid candidate.
		d = math.Inf(1)
	}
	e.update(pos, col, d)
	return col, d, true
}

func (e *rlEvaluator) eval(pos float64) (float64, bool) {
	_, d, ok := e.probe(pos)
	return d, ok
}

// update records a candidate. It keeps the leftmost position among all
// candidates whose distance lies within rlTieEpsilon of the smallest distance
// observed. Tracking the global minimum separately (instead of comparing to
// the currently stored best) prevents the accepted band from drifting upwards.
func (e *rlEvaluator) update(pos float64, col Color, d float64) {
	if !e.best.found {
		e.minDist = d
		e.best = rlCandidate{position: pos, color: col, distance: d, found: true}
		return
	}

	if d < e.minDist {
		e.minDist = d
	}

	inBand := d <= e.minDist+rlTieEpsilon
	bestInBand := e.best.distance <= e.minDist+rlTieEpsilon

	switch {
	case inBand && (!bestInBand || pos < e.best.position):
		e.best = rlCandidate{position: pos, color: col, distance: d, found: true}
	case !inBand && d < e.best.distance-rlTieEpsilon:
		// Clearly better than the incumbent but, because the global minimum
		// is even smaller, outside the tie band. This can only happen while
		// the incumbent is stale; replace it so the result stays close.
		e.best = rlCandidate{position: pos, color: col, distance: d, found: true}
	}
}

func rlRGBDistance(a, b Color) float64 {
	dr := a.R - b.R
	dg := a.G - b.G
	db := a.B - b.B
	da := a.A - b.A
	return math.Sqrt(dr*dr + dg*dg + db*db + da*da)
}

// rlKind selects the segment search strategy.
type rlKind int

const (
	rlKindSampled rlKind = iota
	rlKindLinear
	rlKindSmoothstep
	rlKindSharp
)

type rlStructure struct {
	positions []float64
	colors    [][4]float64 // converted stop colors, used by analytic solvers
	kind      rlKind
}

// rlStructureOf extracts the stop table of the built-in gradient types. It
// reports false for custom cores and when the wrapper domain does not match
// the core's own domain.
func rlStructureOf(g Gradient) (rlStructure, bool) {
	switch core := g.Core.(type) {
	case *LinearGradient:
		if core.min == g.Min && core.max == g.Max {
			if core.mode == BlendRgb {
				return rlStructure{positions: core.positions, colors: core.colors, kind: rlKindLinear}, true
			}
			return rlStructure{positions: core.positions, kind: rlKindSampled}, true
		}
	case *SmoothstepGradient:
		if core.min == g.Min && core.max == g.Max {
			if core.mode == BlendRgb {
				return rlStructure{positions: core.positions, colors: core.colors, kind: rlKindSmoothstep}, true
			}
			return rlStructure{positions: core.positions, kind: rlKindSampled}, true
		}
	case *BasisGradient:
		if core.min == g.Min && core.max == g.Max {
			return rlStructure{positions: core.positions, kind: rlKindSampled}, true
		}
	case *CatmullRomGradient:
		if core.min == g.Min && core.max == g.Max {
			return rlStructure{positions: core.positions, kind: rlKindSampled}, true
		}
	case SharpGradient:
		if core.min == g.Min && core.max == g.Max {
			return rlStructure{positions: core.positions, kind: rlKindSharp}, true
		}
	case *SharpGradient:
		if core.min == g.Min && core.max == g.Max {
			return rlStructure{positions: core.positions, kind: rlKindSharp}, true
		}
	}
	return rlStructure{}, false
}

// solveQuadraticSegment minimizes the RGBA distance on a segment whose output
// is C(s) = a + (b-a)*s with s in [0,1]. For a linear segment s = u; for a
// smoothstep segment s = 3u^2 - 2u^3 and the minimizing s is inverted back to
// u. The squared distance is a quadratic in s, hence solved in closed form.
func (e *rlEvaluator) solveQuadraticSegment(a, b [4]float64, lo, hi float64, smoothstep bool) {
	target := [4]float64{e.target.R, e.target.G, e.target.B, e.target.A}

	var aa, bb float64
	for k := 0; k < 4; k++ {
		d := b[k] - a[k]
		q := a[k] - target[k]
		aa += d * d
		bb += q * d
	}

	s := 0.0
	if aa > 0 {
		s = rlClamp01(-bb / aa)
	}

	u := s
	if smoothstep {
		u = rlSmoothstepInverse(s)
	}
	e.eval(lo + u*(hi-lo))
}

// rlSmoothstepInverse inverts s = 3u^2 - 2u^3 on [0,1], which is strictly
// monotone there. Bisection needs no core evaluations.
func rlSmoothstepInverse(s float64) float64 {
	s = rlClamp01(s)
	if s <= 0 {
		return 0
	}
	if s >= 1 {
		return 1
	}
	lo, hi := 0.0, 1.0
	for i := 0; i < 48; i++ {
		mid := (lo + hi) * 0.5
		v := mid * mid * (3 - 2*mid)
		if v < s {
			lo = mid
		} else {
			hi = mid
		}
	}
	return (lo + hi) * 0.5
}

// searchSharpSegment handles one interval of a [SharpGradient]. Even indices
// are flat bands, odd indices are linear blend ramps.
func (e *rlEvaluator) searchSharpSegment(i int, lo, hi float64) {
	if i%2 == 0 {
		// Flat color band. Probe both the boundary and a point just inside,
		// because At at a stop resolves to the interval on its left.
		e.eval(lo)
		if next := math.Nextafter(lo, math.Inf(1)); next < hi {
			e.eval(next)
		}
		e.eval(hi)
		return
	}

	ca, _, ok := e.probe(math.Nextafter(lo, math.Inf(1)))
	if !ok {
		return
	}
	cb, _, ok := e.probe(hi)
	if !ok {
		return
	}
	e.solveQuadraticSegment(
		[4]float64{ca.R, ca.G, ca.B, ca.A},
		[4]float64{cb.R, cb.G, cb.B, cb.A},
		lo, hi, false,
	)
}

// sampleSegment densely samples one known segment and refines every local
// minimum it contains. The refinement allowance is shared so a segment with
// many minima cannot spend without limit.
func (e *rlEvaluator) sampleSegment(lo, hi float64) {
	distances := make([]float64, rlSegmentSamples+1)
	step := (hi - lo) / float64(rlSegmentSamples)
	for j := 0; j <= rlSegmentSamples; j++ {
		d, ok := e.eval(lo + step*float64(j))
		if !ok {
			return
		}
		distances[j] = d
	}

	brackets := rlCollectBrackets(lo, step, distances)
	if len(brackets) == 0 {
		return
	}

	// Spend at most ~3 full refinements' worth of probes on one segment,
	// divided across its minima, with a small floor for precision.
	iters := (3*(rlRefineIterations+2))/len(brackets) - 2
	if iters < rlMinRefineIterations {
		iters = rlMinRefineIterations
	}
	if iters > rlRefineIterations {
		iters = rlRefineIterations
	}
	for _, br := range brackets {
		if e.exhausted() {
			return
		}
		e.refine(br.lo, br.hi, iters)
	}
}

// scanGlobal runs a uniform grid over the whole domain and refines every
// local minimum found, sharing a fixed refinement budget. Used for unknown or
// wrapped cores and very large stop tables.
func (e *rlEvaluator) scanGlobal() {
	lo, hi := e.g.Min, e.g.Max
	n := rlGlobalSamples
	if n+2 > e.budget {
		n = e.budget - 2
	}
	if n < 2 {
		e.eval((lo + hi) * 0.5)
		return
	}

	distances := make([]float64, n+1)
	step := (hi - lo) / float64(n)
	for j := 0; j <= n; j++ {
		d, ok := e.eval(lo + step*float64(j))
		if !ok {
			return
		}
		distances[j] = d
	}

	brackets := rlCollectBrackets(lo, step, distances)
	if len(brackets) == 0 {
		return
	}

	iters := (rlGlobalRefineBudget)/len(brackets) - 2
	if iters < rlMinRefineIterations {
		iters = rlMinRefineIterations
	}
	if iters > rlRefineIterations {
		iters = rlRefineIterations
	}
	for _, br := range brackets {
		if e.exhausted() {
			return
		}
		e.refine(br.lo, br.hi, iters)
	}
}

type rlBracket struct {
	lo, hi   float64
	distance float64
}

// rlCollectBrackets turns a uniform distance grid into refinement intervals:
// every interior local minimum plus a bracket around the global grid minimum.
// Intervals stay in left-to-right order.
func rlCollectBrackets(origin, step float64, d []float64) []rlBracket {
	n := len(d) - 1
	if n < 1 {
		return nil
	}

	global := 0
	for j := 1; j <= n; j++ {
		if d[j] < d[global] {
			global = j
		}
	}

	var brackets []rlBracket
	for j := 1; j < n; j++ {
		if d[j] <= d[j-1] && d[j] <= d[j+1] {
			brackets = append(brackets, rlBracket{
				lo:       origin + step*float64(j-1),
				hi:       origin + step*float64(j+1),
				distance: d[j],
			})
		}
	}

	// Always bracket the global minimum as well; it may sit at an edge or not
	// qualify as an interior local minimum.
	from := global - 1
	to := global + 1
	if from < 0 {
		from = 0
	}
	if to > n {
		to = n
	}
	brackets = append(brackets, rlBracket{
		lo:       origin + step*float64(from),
		hi:       origin + step*float64(to),
		distance: d[global],
	})

	// Merge overlapping/duplicate brackets so refinement is not repeated.
	sort.SliceStable(brackets, func(i, j int) bool {
		if brackets[i].lo != brackets[j].lo {
			return brackets[i].lo < brackets[j].lo
		}
		return brackets[i].hi < brackets[j].hi
	})
	merged := brackets[:0]
	for _, br := range brackets {
		if len(merged) == 0 || br.lo > merged[len(merged)-1].hi {
			merged = append(merged, br)
			continue
		}
		if br.hi > merged[len(merged)-1].hi {
			merged[len(merged)-1].hi = br.hi
		}
		if br.distance < merged[len(merged)-1].distance {
			merged[len(merged)-1].distance = br.distance
		}
	}
	return merged
}

// refine minimizes on [lo, hi] with at most iterations golden-section steps.
// Every probe is fed to update, so the globally smallest (and leftmost on
// ties) candidate is retained even when the bracket is not perfectly unimodal.
func (e *rlEvaluator) refine(lo, hi float64, iterations int) {
	if !(hi > lo) || iterations <= 0 {
		return
	}

	const invPhi = 0.3819660112501051 // (3 - sqrt(5)) / 2

	x1 := lo + invPhi*(hi-lo)
	x2 := hi - invPhi*(hi-lo)

	d1, ok := e.eval(x1)
	if !ok {
		return
	}
	d2, ok := e.eval(x2)
	if !ok {
		return
	}

	for i := 0; i < iterations; i++ {
		if hi-lo <= rlPosEpsilon || e.exhausted() {
			return
		}
		if d1 < d2 {
			hi, x2, d2 = x2, x1, d1
			x1 = lo + invPhi*(hi-lo)
			d1, ok = e.eval(x1)
		} else {
			lo, x1, d1 = x1, x2, d2
			x2 = hi - invPhi*(hi-lo)
			d2, ok = e.eval(x2)
		}
		if !ok {
			return
		}
	}
}

func rlClamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
