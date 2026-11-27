# Changelog

## Unreleased

- New `Gradient.PerceptualPalette()`: sample colors at equal perceptual
  distance (Oklab) instead of equal parameter steps, returning each color with
  its domain position
- New `Gradient.PerceptualPaletteXY()` / `Gradient.PerceptualColors()` for
  parallel color and position slices
- New `ColorDistance()` and `ColorDistanceWeighted()` helpers

## v0.11.1

- Fix bug in Lab conversion

## v0.11.0

- New `GradientBuilder.Reset()`
- New `BlendLab`
- `Gradient`'s fields is now public
- `GradientCore` is now public
- New `InterpolationSmoothstep`

## v0.10.2

- Fix bug in `GradientBuilder.Css()`

## v0.10.1

- New `GradientBuilder.Css()`
- Using `uint32` to store preset colors

## v0.10.0

- Added support for transparency
- New `GoColor()`
- Refactor lots of code
- Remove color schemes
