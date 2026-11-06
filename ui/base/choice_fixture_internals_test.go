package base

func chosenValue[T any](field *Select[T]) T { chosen, _ := field.Chosen(); return chosen.Value }
