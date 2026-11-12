package base

import (
	"strconv"
	"testing"
)

func TestCommandRecencyReleasesAnEvictedName(t *testing.T) {
	var commands Commands[struct{}]
	for index := range 17 {
		name := strconv.Itoa(index)
		commands.Add(Command{Name: name}, struct{}{})
		commands.Used(name)
	}
	if len(commands.used) != 16 {
		t.Fatalf("recent command count = %d, want 16", len(commands.used))
	}
	commands.Remove("0")
	for _, name := range commands.used[:cap(commands.used)] {
		if name == "0" {
			t.Fatal("evicted command name remains reachable through recency storage")
		}
	}
}
