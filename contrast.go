package colorgrad

import (
	"errors"
	"fmt"
	"math"
	"sort"
)

// WCAG 2.x relative-luminance constants. The 0.03928 cutoff is the value
// printed in WCAG 2.x; 0.04045 is the errata-corrected sRGB boundary, but the
// difference is far below the precision used by this package.
const (
	wcagLinearCutoff = 0.03928
	wcagDarkDivisor  = 12.92
	wcagOffset       = 0.055
	wcagScale        = 1.055
	wcagGamma        = 2.4
	wcagLumR         = 0.2126
	wcagLumG         = 0.7152
	wcagLumB         = 0.0722
	wcagFloor        = 0.05
)

// Search bounds. The coarse pass keeps the query independent of the concrete
// GradientCore implementation; the bounded golden-section passes refine the
// best intervals without recursing without limit.
const (
	contrastDefaultSamples    = 1024
	contrastSegmentSamples    = 24
	contrastRefineCells       = 24
	contrastGoldenIterations  = 64
	contrastRatioTieEpsilon   = 1e-12
	contrastPositionTieFactor = 1e-7
)

// Errors returned by contrast queries.
var (
	ErrContrastQuery   = errors.New("colorgrad: invalid contrast query")
	ErrInvalidColor    = fmt.Errorf("%w: invalid color", ErrContrastQuery)
	ErrInvalidDomain   = fmt.Errorf("%w: invalid domain", ErrContrastQuery)
	ErrInvalidGradient = fmt.Errorf("%w: invalid gradient", ErrContrastQuery)
	ErrNonFiniteValue  = fmt.Errorf("%w: non-finite value", ErrContrastQuery)
	ErrContrastSearch  = fmt.Errorf("%w: contrast search failed", ErrContrastQuery)
)

// ContrastResult is the least-readable position in a gradient for a given
// text color and opaque background.
type ContrastResult struct {
	// Ratio is the WCAG 2.x contrast ratio at Position.
	Ratio float64

	// Position is in Gradient.Domain coordinates. When several positions are
	// equal minima, the leftmost stable representative is returned.
	Position float64
}

// RelativeLuminance returns the WCAG 2.x relative luminance of an sRGB color.
// Out-of-gamut finite colors are clamped as they would be for display.
func RelativeLuminance(c Color) float64 {
	c = c.Clamp()
	r := srgbToLinear(c.R)
	g := srgbToLinear(c.G)
	b := srgbToLinear(c.B)
	return wcagLumR*r + wcagLumG*g + wcagLumB*b
}

// ContrastRatio returns the WCAG 2.x contrast ratio between two colors.
func ContrastRatio(a, b Color) float64 {
	return luminanceContrast(RelativeLuminance(a), RelativeLuminance(b))
}

// MinContrast returns the smallest WCAG 2.x text/background contrast over
// the gradient's domain and its position.
//
// Gradient and text alpha are composited with source-over against the opaque
// background before luminance is calculated. The receiver gradient is not
// modified, and Gradient.At keeps its existing behavior.
func (g Gradient) MinContrast(text, background Color) (ContrastResult, error) {
	return minContrast(g, text, background)
}

// MinContrast is the package-function form of Gradient.MinContrast.
func MinContrast(g Gradient, text, background Color) (ContrastResult, error) {
	return minContrast(g, text, background)
}

// MinContrastXY is a convenience form for callers that want the ratio and
// position directly.
func (g Gradient) MinContrastXY(text, background Color) (ratio, position float64, err error) {
	res, err := g.MinContrast(text, background)
	if err != nil {
		return 0, 0, err
	}
	return res.Ratio, res.Position, nil
}

// MinContrastXY is the package-function form of Gradient.MinContrastXY.
func MinContrastXY(g Gradient, text, background Color) (ratio, position float64, err error) {
	res, err := MinContrast(g, text, background)
	if err != nil {
		return 0, 0, err
	}
	return res.Ratio, res.Position, nil
}

func minContrast(g Gradient, text, background Color) (ContrastResult, error) {
	if err := validateQueryColor(text, false); err != nil {
		return ContrastResult{}, err
	}
	if err := validateQueryColor(background, true); err != nil {
		return ContrastResult{}, err
	}
	if g.Core == nil {
		return ContrastResult{}, fmt.Errorf("%w: nil GradientCore", ErrInvalidGradient)
	}
	if !finiteFloat(g.Min) || !finiteFloat(g.Max) {
		return ContrastResult{}, fmt.Errorf("%w: min=%v max=%v", ErrNonFiniteValue, g.Min, g.Max)
	}
	if !(g.Max > g.Min) {
		return ContrastResult{}, fmt.Errorf("%w: min=%v max=%v", ErrInvalidDomain, g.Min, g.Max)
	}
	width := g.Max - g.Min
	if !finiteFloat(width) || width <= 0 {
		return ContrastResult{}, fmt.Errorf("%w: width=%v", ErrInvalidDomain, width)
	}

	s := &contrastSearch{
		g:          g,
		text:       text,
		background: background,
		cache:      make(map[float64]float64),
	}

	points := contrastSamplePoints(g)
	var best ContrastResult
	bestIndex := -1

	for i, t := range points {
		r, err := s.ratioAt(t)
		if err != nil {
			return ContrastResult{}, err
		}
		if bestIndex < 0 || contrastLess(r, t, best.Ratio, best.Position, width) {
			best = ContrastResult{Ratio: r, Position: t}
			bestIndex = i
		}
	}
	if bestIndex < 0 {
		return ContrastResult{}, fmt.Errorf("%w: no sample points", ErrContrastSearch)
	}

	// Refine a bounded number of the most promising coarse intervals. Every
	// known stop is already a point, so these intervals do not cross a
	// gradient breakpoint.
	type cell struct {
		lo, hi float64
		score  float64
		order  int
	}
	cells := make([]cell, 0, len(points)-1)
	for i := 0; i+1 < len(points); i++ {
		lo, hi := points[i], points[i+1]
		if hi <= lo {
			continue
		}
		rl, err := s.ratioAt(lo)
		if err != nil {
			return ContrastResult{}, err
		}
		rh, err := s.ratioAt(hi)
		if err != nil {
			return ContrastResult{}, err
		}
		cells = append(cells, cell{lo: lo, hi: hi, score: math.Min(rl, rh), order: i})
	}

	ranked := append([]cell(nil), cells...)
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score < ranked[j].score
		}
		return ranked[i].order < ranked[j].order
	})

	selectedOrders := make(map[int]struct{}, contrastRefineCells+2)
	for _, idx := range []int{bestIndex - 1, bestIndex} {
		if idx >= 0 && idx < len(cells) {
			selectedOrders[idx] = struct{}{}
		}
	}
	for _, c := range ranked {
		if len(selectedOrders) >= contrastRefineCells {
			break
		}
		selectedOrders[c.order] = struct{}{}
	}

	selected := make([]cell, 0, len(selectedOrders))
	for _, c := range cells {
		if _, ok := selectedOrders[c.order]; ok {
			selected = append(selected, c)
		}
	}

	for _, c := range selected {
		x, r, err := goldenMinimize(s, c.lo, c.hi)
		if err != nil {
			return ContrastResult{}, err
		}
		x = clampDomain(x, g.Min, g.Max)
		r, err = s.ratioAt(x)
		if err != nil {
			return ContrastResult{}, err
		}
		if contrastLess(r, x, best.Ratio, best.Position, width) {
			best = ContrastResult{Ratio: r, Position: x}
		}
	}

	// Report a value computed at the returned position.
	r, err := s.ratioAt(best.Position)
	if err != nil {
		return ContrastResult{}, err
	}
	best.Ratio = r
	return best, nil
}

type contrastSearch struct {
	g          Gradient
	text       Color
	background Color
	cache      map[float64]float64
}

func (s *contrastSearch) ratioAt(t float64) (ratio float64, err error) {
	if r, ok := s.cache[t]; ok {
		return r, nil
	}

	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("%w: GradientCore.At panicked at %v: %v", ErrContrastSearch, t, rec)
		}
		if err != nil {
			return
		}
		if !finiteFloat(ratio) || ratio < 1-1e-12 {
			err = fmt.Errorf("%w: invalid contrast ratio %v at %v", ErrNonFiniteValue, ratio, t)
			return
		}
		s.cache[t] = ratio
	}()

	raw := s.g.At(t)
	if !finiteColor(raw, true) {
		return 0, fmt.Errorf("%w: gradient color at %v is %+v", ErrNonFiniteValue, t, raw)
	}
	c := raw.Clamp()

	backdrop := compositeOver(c, s.background)
	if !finiteColor(backdrop, false) {
		return 0, fmt.Errorf("%w: invalid composited backdrop at %v", ErrNonFiniteValue, t)
	}
	foreground := compositeOver(s.text, backdrop)
	if !finiteColor(foreground, false) {
		return 0, fmt.Errorf("%w: invalid composited text color at %v", ErrNonFiniteValue, t)
	}

	lf := RelativeLuminance(foreground)
	lb := RelativeLuminance(backdrop)
	if !finiteFloat(lf) || !finiteFloat(lb) {
		return 0, fmt.Errorf("%w: non-finite luminance at %v", ErrNonFiniteValue, t)
	}
	ratio = luminanceContrast(lf, lb)
	return ratio, nil
}

func contrastSamplePoints(g Gradient) []float64 {
	seen := make(map[float64]struct{})
	add := func(t float64) {
		if !finiteFloat(t) {
			return
		}
		if t < g.Min {
			t = g.Min
		} else if t > g.Max {
			t = g.Max
		}
		seen[t] = struct{}{}
	}

	add(g.Min)
	add(g.Max)

	for i := 0; i <= contrastDefaultSamples; i++ {
		u := float64(i) / contrastDefaultSamples
		add(lerpFinite(g.Min, g.Max, u))
	}

	stops := contrastStops(g.Core)
	validStops := make([]float64, 0, len(stops)+2)
	validStops = append(validStops, g.Min)
	for _, p := range stops {
		if finiteFloat(p) && p >= g.Min && p <= g.Max {
			add(p)
			validStops = append(validStops, p)
		}
	}
	validStops = append(validStops, g.Max)
	sort.Float64s(validStops)

	for i := 0; i+1 < len(validStops); i++ {
		a, b := validStops[i], validStops[i+1]
		if b <= a {
			continue
		}
		// Subdivide each concrete stop interval. This is what makes narrow
		// spline overshoots and sharp transition bands visible to the coarse
		// pass without scanning the entire domain at that density.
		for j := 1; j < contrastSegmentSamples; j++ {
			u := float64(j) / contrastSegmentSamples
			add(lerpFinite(a, b, u))
		}
	}

	points := make([]float64, 0, len(seen))
	for t := range seen {
		points = append(points, t)
	}
	sort.Float64s(points)
	return points
}

func contrastStops(core GradientCore) []float64 {
	switch c := core.(type) {
	case LinearGradient:
		return c.positions
	case *LinearGradient:
		return c.positions
	case SmoothstepGradient:
		return c.positions
	case *SmoothstepGradient:
		return c.positions
	case *BasisGradient:
		return c.positions
	case CatmullRomGradient:
		return c.positions
	case *CatmullRomGradient:
		return c.positions
	case SharpGradient:
		return c.positions
	case *SharpGradient:
		return c.positions
	default:
		return nil
	}
}

func goldenMinimize(s *contrastSearch, lo, hi float64) (x float64, r float64, err error) {
	const invPhi = 0.6180339887498948 // (sqrt(5)-1)/2
	a, b := lo, hi
	c := b - invPhi*(b-a)
	d := a + invPhi*(b-a)
	fc, err := s.ratioAt(c)
	if err != nil {
		return 0, 0, err
	}
	fd, err := s.ratioAt(d)
	if err != nil {
		return 0, 0, err
	}

	for i := 0; i < contrastGoldenIterations; i++ {
		// On an exact tie, retain the left bracket. This gives flat or
		// symmetric minima a deterministic leftward bias.
		if fc <= fd {
			b = d
			d = c
			fd = fc
			c = b - invPhi*(b-a)
			fc, err = s.ratioAt(c)
		} else {
			a = c
			c = d
			fc = fd
			d = a + invPhi*(b-a)
			fd, err = s.ratioAt(d)
		}
		if err != nil {
			return 0, 0, err
		}
	}

	x = (a + b) / 2
	r, err = s.ratioAt(x)
	if err != nil {
		return 0, 0, err
	}
	// Prefer a directly evaluated golden point if the midpoint rounding is
	// slightly worse.
	if fc < r {
		x, r = c, fc
	}
	if fd < r {
		x, r = d, fd
	}
	return x, r, nil
}

func contrastLess(candidateRatio, candidatePos, bestRatio, bestPos, domainWidth float64) bool {
	if candidateRatio < bestRatio-contrastRatioTieEpsilon {
		return true
	}
	if candidateRatio > bestRatio+contrastRatioTieEpsilon {
		return false
	}
	posEps := contrastPositionTieFactor * math.Max(math.Abs(domainWidth), 1)
	// Within the ratio tie tolerance, keep the earlier representative unless
	// the candidate is materially to the left. This snaps numerical noise back
	// to an exact stop while still resolving genuinely separate equal minima.
	return candidatePos < bestPos-posEps
}

func validateQueryColor(c Color, opaqueBackground bool) error {
	if !finiteColor(c, true) {
		return fmt.Errorf("%w: %+v", ErrNonFiniteValue, c)
	}
	if c.R < 0 || c.R > 1 || c.G < 0 || c.G > 1 || c.B < 0 || c.B > 1 || c.A < 0 || c.A > 1 {
		return fmt.Errorf("%w: components must be within [0,1], got %+v", ErrInvalidColor, c)
	}
	if opaqueBackground && c.A != 1 {
		return fmt.Errorf("%w: background must be opaque (A=1), got A=%v", ErrInvalidColor, c.A)
	}
	return nil
}

func finiteColor(c Color, allowAlphaOvershoot bool) bool {
	if !finiteFloat(c.R) || !finiteFloat(c.G) || !finiteFloat(c.B) || !finiteFloat(c.A) {
		return false
	}
	if allowAlphaOvershoot {
		return true
	}
	return c.R >= 0 && c.R <= 1 && c.G >= 0 && c.G <= 1 && c.B >= 0 && c.B <= 1 && c.A >= 0 && c.A <= 1
}

func finiteFloat(f float64) bool {
	return !math.IsNaN(f) && !math.IsInf(f, 0)
}

func srgbToLinear(c float64) float64 {
	// Query colors are validated and gradient colors are clamped before this
	// is called. The branch follows WCAG 2.x.
	if c <= wcagLinearCutoff {
		return c / wcagDarkDivisor
	}
	return math.Pow((c+wcagOffset)/wcagScale, wcagGamma)
}

func luminanceContrast(l1, l2 float64) float64 {
	light, dark := l1, l2
	if light < dark {
		light, dark = dark, light
	}
	return (light + wcagFloor) / (dark + wcagFloor)
}

func compositeOver(src, dst Color) Color {
	sa := clamp01(src.A)
	da := clamp01(dst.A)
	outA := sa + da*(1-sa)
	if outA <= 0 {
		return Color{}
	}
	r := (clamp01(src.R)*sa + clamp01(dst.R)*da*(1-sa)) / outA
	g := (clamp01(src.G)*sa + clamp01(dst.G)*da*(1-sa)) / outA
	b := (clamp01(src.B)*sa + clamp01(dst.B)*da*(1-sa)) / outA
	return Color{R: r, G: g, B: b, A: outA}
}

func lerpFinite(a, b, u float64) float64 {
	if u <= 0.5 {
		return a + (b-a)*u
	}
	return b - (b-a)*(1-u)
}

func clampDomain(t, lo, hi float64) float64 {
	if t < lo {
		return lo
	}
	if t > hi {
		return hi
	}
	return t
}
