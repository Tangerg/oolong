# Run the examples

The canonical [example catalog](../docs/guides/examples.md) orders every command by prerequisite, learning goal, controls, and public API. Its [Chinese translation](../docs/zh/guides/examples.md) follows the same executable path.

Run one command from the repository root:

```sh
go run ./examples/hello
```

To preview a PNG in a Kitty graphics terminal, press `1` for the full image, `2` or `3` for either half, and `q` to quit.

```sh
go run ./examples/image docs/public/brand/oolong-icon.png
```

Test the complete example module with:

```sh
cd examples
go test ./...
```

Every command has a neighboring `main_test.go`. The catalog test derives command names from these directories and rejects missing or duplicated documentation entries.
