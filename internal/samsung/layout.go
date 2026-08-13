// Package samsung is the future Samsung Messages UI adapter boundary.
package samsung

type Point struct{ X, Y int }
type Layout struct {
	Width, Height  int
	Composer, Send Point
}

func DefaultLayout() Layout { return Layout{1080, 1920, Point{500, 1800}, Point{1004, 1273}} }
