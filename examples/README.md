# Run the examples

The canonical [example catalog](../docs/examples.md) orders every command by prerequisite, learning goal, controls, and public API. Its [Chinese translation](../docs/zh/examples.md) follows the same executable path.

Run one command from the repository root:

```sh
go run ./examples/hello
```

Test the complete example module with:

```sh
cd examples
go test ./...
```

Every command has a neighboring `main_test.go`. The catalog test derives command names from these directories and rejects missing or duplicated documentation entries.
