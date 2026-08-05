package layout_test

import (
	"fmt"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/layout"
)

func ExampleRows() {
	// The order of business is measure, then arrange. Nothing is drawn: the caller
	// draws into the views it is handed, which is what lets a slot be left empty.
	views := layout.Rows(grid.NewSurface(20, 10).View(),
		layout.Slot{Size: layout.Fixed(1)},
		layout.Slot{Size: layout.Flex(1)},
		layout.Slot{Size: layout.Fixed(2)},
	)
	for i, v := range views {
		_, h := v.Size()
		fmt.Printf("slot %d: %d rows\n", i, h)
	}

	// Output:
	// slot 0: 1 rows
	// slot 1: 7 rows
	// slot 2: 2 rows
}

func ExampleMeasured() {
	// A measured slot is asked about the axis being divided, given the room across
	// the other one — so Measured means the same thing in a row and in a column.
	wide := layout.MeasureFunc(func(across int) int { return across / 4 })
	rows := layout.Rows(grid.NewSurface(20, 10).View(),
		layout.Slot{Size: layout.Measured(0, 0), Of: wide},
		layout.Slot{Size: layout.Flex(1)},
	)
	cols := layout.Columns(grid.NewSurface(20, 8).View(),
		layout.Slot{Size: layout.Measured(0, 0), Of: wide},
		layout.Slot{Size: layout.Flex(1)},
	)
	_, h := rows[0].Size()
	w, _ := cols[0].Size()
	fmt.Printf("measured against a width of 20: %d rows\n", h)
	fmt.Printf("measured against a height of 8: %d columns\n", w)

	// Output:
	// measured against a width of 20: 5 rows
	// measured against a height of 8: 2 columns
}
