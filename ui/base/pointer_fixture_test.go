package base_test

import (
	"image"

	"github.com/Tangerg/oolong/ui/core/input"
)

func regionPress(x, y int) input.Mouse {
	return input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(x, y)}
}

func regionDrag(x, y int) input.Mouse {
	return input.Mouse{Action: input.MouseDrag, Button: input.ButtonLeft, Pos: image.Pt(x, y)}
}

func regionRelease(x, y int) input.Mouse {
	return input.Mouse{Action: input.MouseUp, Button: input.ButtonLeft, Pos: image.Pt(x, y)}
}
