package layout_test

import (
	"fmt"
	"image"

	"github.com/Tangerg/oolong/core/layout"
)

func ExampleAxis_Rects() {
	// The order of business is measure, then arrange. The caller projects the
	// rectangles into whatever coordinate model it uses.
	rects := layout.Down.Rects(image.Pt(20, 10),
		layout.Slot{Size: layout.Fixed(1)},
		layout.Slot{Size: layout.Flex(1)},
		layout.Slot{Size: layout.Fixed(2)},
	)
	for i, r := range rects {
		fmt.Printf("slot %d: %d units\n", i, r.Dy())
	}

	// Output:
	// slot 0: 1 units
	// slot 1: 7 units
	// slot 2: 2 units
}

func ExampleMeasured() {
	// A measured slot is asked about the axis being divided, given the room across
	// the other one — so Measured means the same thing in a row and in a column.
	wide := layout.MeasureFunc(func(across int) int { return across / 4 })
	rows := layout.Down.Rects(image.Pt(20, 10),
		layout.Slot{Size: layout.Measured(0, 0), Of: wide},
		layout.Slot{Size: layout.Flex(1)},
	)
	cols := layout.Across.Rects(image.Pt(20, 8),
		layout.Slot{Size: layout.Measured(0, 0), Of: wide},
		layout.Slot{Size: layout.Flex(1)},
	)
	fmt.Printf("measured against a width of 20: %d rows\n", rows[0].Dy())
	fmt.Printf("measured against a height of 8: %d columns\n", cols[0].Dx())

	// Output:
	// measured against a width of 20: 5 rows
	// measured against a height of 8: 2 columns
}

func ExampleFlow() {
	// A gap belongs to the division rather than to every region but the last. Cross
	// placement independently constrains and aligns a region on the other axis.
	rows := layout.Down.Rects(image.Pt(24, 4),
		layout.Slot{Size: layout.Flex(1)},
		layout.Slot{Size: layout.Fixed(1), Cross: layout.Cross{Size: 8, Align: layout.Center}},
	)
	regions := (layout.Flow{Axis: layout.Across, Gap: 2}).Rects(rows[0].Size(), []layout.Slot{
		{Size: layout.Part(1, 2)},
		{Size: layout.Flex(1)},
		{Size: layout.Flex(1)},
	})
	for _, region := range regions {
		fmt.Printf("region %dx%d\n", region.Dx(), region.Dy())
	}
	fmt.Printf("aligned %d wide\n", rows[1].Dx())

	// Output:
	// region 10x3
	// region 5x3
	// region 5x3
	// aligned 8 wide
}
