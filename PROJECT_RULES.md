# Repository instructions

Engineering conventions that apply to every change live in [`AGENTS.md`](AGENTS.md). The rules below hold only
for this repository, and each one resolves a question `AGENTS.md` deliberately leaves open.

- **Backward compatibility is not a goal during development.** Remove obsolete paths instead of adding
  compatibility layers, fallbacks, or migrations. Make architectural decisions for the long term while
  breaking changes are still inexpensive, and do not accept a stopgap that is meant to be replaced later.
- **Grow the system in layers.** Start from the smallest version that works end to end, and add each new
  capability on top of a product that already works. Never trade a working product for unfinished complexity.
- **Repository-local usage is no evidence for or against a public API.** Retain or remove exported operations
  and extension points by responsibility, abstraction quality, and downstream utility, never merely because
  code in this repository does or does not call them.
- **Use explicit `Config` structs** for related construction settings, and give optional fields useful zero
  meanings. Do not introduce functional-options APIs.
- **The import graph owns the layering, and inventories are derived.** `internal/arch` reads Go's package
  facts directly rather than consulting a maintained list, and `scripts/modules.sh` is the executable module
  inventory. Never add a second hand-maintained count, catalog, or exclusion list beside a derived one.
