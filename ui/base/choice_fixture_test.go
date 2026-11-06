package base_test

import "github.com/Tangerg/oolong/ui/base"

func chosenValue[T any](field *base.Select[T]) T { chosen, _ := field.Chosen(); return chosen.Value }
