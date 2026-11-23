package layout

// Measurer reports the desired extent along axis given the available cross extent.
// Down asks for height at a width; Across asks for width at a height. Implementations
// must interpret the explicit axis, so height-only content cannot satisfy this seam.
type Measurer interface {
	Measure(axis Axis, across int) int
}

type MeasureFunc func(axis Axis, across int) int

// MeasureRows measures content that fills whatever width it is given and needs
// height(width) rows for it. It has no intrinsic width, so a flow along the
// horizontal axis leaves its size to the slot rather than to the content.
func MeasureRows(height func(width int) int) MeasureFunc {
	return func(axis Axis, width int) int {
		if axis == Down {
			return height(width)
		}
		return 0
	}
}

// Measure panics for an invalid axis before invoking f, so the callback only
// receives a supported dimension.
func (f MeasureFunc) Measure(axis Axis, across int) int {
	if err := axis.Validate(); err != nil {
		panic(err)
	}
	return f(axis, across)
}
