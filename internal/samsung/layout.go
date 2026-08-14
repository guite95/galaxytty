// Package samsung controls Samsung Messages on a scrcpy virtual display.
package samsung

import "fmt"

type Point struct{ X, Y int }
type Layout struct {
	Width, Height  int
	Composer, Send Point
}

func DefaultLayout() Layout { return Layout{1080, 1920, Point{500, 1800}, Point{1004, 955}} }

func (l Layout) Validate() error {
	if l.Width <= 0 || l.Height <= 0 {
		return fmt.Errorf("Samsung display dimensions must be positive")
	}
	for _, item := range []struct {
		name  string
		point Point
	}{{"composer", l.Composer}, {"send", l.Send}} {
		if item.point.X < 0 || item.point.X >= l.Width || item.point.Y < 0 || item.point.Y >= l.Height {
			return fmt.Errorf("Samsung %s coordinate is outside the display", item.name)
		}
	}
	return nil
}
