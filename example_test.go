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

func ExampleGradient_AverageColor() {
	grad, err := colorgrad.NewGradient().
		HtmlColors("#f00", "#0f0").
		Build()
	if err != nil {
		panic(err)
	}
	sharp := grad.Sharp(2, 0)

	// Coverage-weighted average over the whole interval: half red, half
	// green, integrated in linear light with premultiplied alpha.
	avg, err := sharp.AverageColor(0, 1)
	if err != nil {
		panic(err)
	}
	fmt.Println(avg.HexString())

	// Bounded precision.
	tight, err := sharp.AverageColor(0, 1,
		colorgrad.AverageEpsilon(1e-10),
		colorgrad.AverageMaxDepth(20),
	)
	if err != nil {
		panic(err)
	}
	fmt.Println(tight.HexString())
	// Output:
	// #bcbc00
	// #bcbc00
}
