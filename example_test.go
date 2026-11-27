package colorgrad_test

import (
	"fmt"

	"github.com/mazznoer/colorgrad"
)

func Example_presetGradient() {
	grad := colorgrad.Rainbow()
	dmin, dmax := grad.Domain()

	fmt.Println(dmin, dmax)
	fmt.Println(grad.At(0).HexString())
	// Output:
	// 0 1
	// #6e40aa
}

func Example_customGradient() {
	grad, err := colorgrad.NewGradient().
		HtmlColors("red", "#FFD700", "lime").
		Domain(0, 0.35, 1).
		Mode(colorgrad.BlendOklab).
		Build()

	if err != nil {
		panic(err)
	}

	fmt.Println(grad.At(0).HexString())
	fmt.Println(grad.At(1).HexString())
	// Output:
	// #ff0000
	// #00ff00
}

func ExampleGradient_PerceptualPalette() {
	grad, err := colorgrad.NewGradient().
		HtmlColors("#002bff", "#00d0ff", "#fff6aa", "#ff2b00").
		Domain(0, 0.7, 0.9, 1).
		Mode(colorgrad.BlendOklab).
		Build()

	if err != nil {
		panic(err)
	}

	// Five bands spread by color change rather than by parameter value.
	palette, err := grad.PerceptualPalette(5, 1)
	if err != nil {
		panic(err)
	}

	for _, entry := range palette {
		fmt.Printf("%.4f %s\n", entry.Position, entry.Color.HexString())
	}
	// Output:
	// 0.0000 #002bff
	// 0.4714 #00a4ff
	// 0.8050 #ace7d8
	// 0.9335 #ffbd7b
	// 1.0000 #ff2b00
}
