package colorgrad

import (
	"fmt"

	"github.com/mazznoer/csscolorparser"
)

type GradientBuilder struct {
	colors             []Color
	positions          []float64
	mode               BlendMode
	interpolation      Interpolation
	invalidHtmlColors  []string
	invalidCssGradient bool
	repeat             bool
	clean              bool
}

func NewGradient() *GradientBuilder {
	return &GradientBuilder{
		mode:               BlendRgb,
		interpolation:      InterpolationLinear,
		invalidCssGradient: false,
		clean:              false,
	}
}

func (gb *GradientBuilder) Colors(colors ...Color) *GradientBuilder {
	for _, col := range colors {
		gb.colors = append(gb.colors, col)
	}
	gb.clean = false
	return gb
}

func (gb *GradientBuilder) HtmlColors(htmlColors ...string) *GradientBuilder {
	for _, s := range htmlColors {
		c, err := csscolorparser.Parse(s)
		if err != nil {
			gb.invalidHtmlColors = append(gb.invalidHtmlColors, s)
			continue
		}
		gb.colors = append(gb.colors, c)
	}
	gb.clean = false
	return gb
}

func (gb *GradientBuilder) Css(s string) *GradientBuilder {
	gb.clean = false
	stops, ok := parseCss(s)
	if !ok {
		gb.invalidCssGradient = true
		return gb
	}
	gb.colors = gb.colors[:0]
	gb.positions = gb.positions[:0]
	for _, st := range stops {
		gb.colors = append(gb.colors, *st.color)
		gb.positions = append(gb.positions, *st.pos)
	}
	gb.repeat = false
	return gb
}

// CssRepeat parses CSS gradient color stops and builds a repeating gradient.
//
// Unlike Css, the stops are not extended to 0% and 100%: the explicit
// positions of the first and last stops define the repeat period, and the
// colors repeat with that period for any position. The first and last stops
// must have explicit, finite positions, positions must be non-decreasing
// (equal positions give hard edges), and the period must be greater than
// zero, otherwise Build reports an invalid CSS gradient error.
func (gb *GradientBuilder) CssRepeat(s string) *GradientBuilder {
	gb.clean = false
	stops, ok := parseCssRepeat(s)
	if !ok {
		gb.invalidCssGradient = true
		return gb
	}
	gb.colors = gb.colors[:0]
	gb.positions = gb.positions[:0]
	for _, st := range stops {
		gb.colors = append(gb.colors, *st.color)
		gb.positions = append(gb.positions, *st.pos)
	}
	gb.repeat = true
	return gb
}

func (gb *GradientBuilder) Domain(positions ...float64) *GradientBuilder {
	gb.positions = positions
	gb.clean = false
	return gb
}

func (gb *GradientBuilder) Mode(mode BlendMode) *GradientBuilder {
	gb.mode = mode
	return gb
}

func (gb *GradientBuilder) Interpolation(mode Interpolation) *GradientBuilder {
	gb.interpolation = mode
	return gb
}

func (gb *GradientBuilder) Reset() *GradientBuilder {
	gb.colors = gb.colors[:0]
	gb.positions = gb.positions[:0]
	gb.mode = BlendRgb
	gb.interpolation = InterpolationLinear
	gb.invalidHtmlColors = gb.invalidHtmlColors[:0]
	gb.invalidCssGradient = false
	gb.repeat = false
	gb.clean = false
	return gb
}

func (gb *GradientBuilder) prepareBuild() error {
	if gb.clean {
		return nil
	}

	if gb.invalidHtmlColors != nil {
		return fmt.Errorf("invalid HTML colors: %q", gb.invalidHtmlColors)
	}

	if gb.invalidCssGradient {
		return fmt.Errorf("invalid CSS gradient")
	}

	var colors []Color
	var positions []float64

	if len(gb.colors) == 0 {
		// Default colors
		colors = []Color{
			{R: 0, G: 0, B: 0, A: 1}, // black
			{R: 1, G: 1, B: 1, A: 1}, // white
		}
	} else if len(gb.colors) == 1 {
		colors = []Color{gb.colors[0], gb.colors[0]}
	} else {
		colors = make([]Color, len(gb.colors))
		copy(colors, gb.colors)
	}

	if len(gb.positions) == 0 {
		positions = linspace(0, 1, uint(len(colors)))
	} else if len(gb.positions) == len(colors) {
		for i := 0; i < len(gb.positions)-1; i++ {
			if gb.positions[i] > gb.positions[i+1] {
				return fmt.Errorf("invalid domain")
			}
		}
		positions = make([]float64, len(gb.positions))
		copy(positions, gb.positions)
	} else if len(gb.positions) == 2 {
		if gb.positions[0] >= gb.positions[1] {
			return fmt.Errorf("invalid domain")
		}
		positions = linspace(gb.positions[0], gb.positions[1], uint(len(colors)))
	} else {
		return fmt.Errorf("invalid domain")
	}

	gb.colors = gb.colors[:0]
	gb.positions = gb.positions[:0]

	prev := positions[0]
	lastIdx := len(positions) - 1

	for i, col := range colors {
		pos := positions[i]
		var next float64
		if i == lastIdx {
			next = positions[lastIdx]
		} else {
			next = positions[i+1]
		}
		if (pos-prev)+(next-pos) < epsilon {
			// skip
		} else {
			gb.colors = append(gb.colors, col)
			gb.positions = append(gb.positions, pos)
		}
		prev = pos
	}

	if len(gb.colors) != len(gb.positions) || len(gb.colors) < 2 {
		return fmt.Errorf("invalid stops")
	}

	gb.clean = true
	return nil
}

func (gb *GradientBuilder) Build() (Gradient, error) {
	if err := gb.prepareBuild(); err != nil {
		return Gradient{
			Core: zeroGradient{},
			Min:  0,
			Max:  1,
		}, err
	}

	var grad Gradient

	if gb.interpolation == InterpolationLinear {
		grad = newLinearGradient(gb.colors, gb.positions, gb.mode)
	} else if gb.interpolation == InterpolationSmoothstep {
		grad = newSmoothstepGradient(gb.colors, gb.positions, gb.mode)
	} else if gb.interpolation == InterpolationBasis {
		grad = newBasisGradient(gb.colors, gb.positions, gb.mode)
	} else {
		grad = newCatmullRomGradient(gb.colors, gb.positions, gb.mode)
	}

	if gb.repeat {
		grad.Core = repeatingGradient{
			core:   grad.Core,
			min:    grad.Min,
			period: grad.Max - grad.Min,
		}
	}

	return grad, nil
}

// For testing purposes
func (gb *GradientBuilder) GetColors() *[]Color {
	return &gb.colors
}

// For testing purposes
func (gb *GradientBuilder) GetPositions() *[]float64 {
	return &gb.positions
}
