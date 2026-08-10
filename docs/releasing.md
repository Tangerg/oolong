---
title: Prepare a coordinated release
description: Validate and publish one immutable version across every public Oolong module.
contentType: How-to
---

# Prepare a coordinated release

Language: English | [简体中文](zh/releasing.md)

This maintainer guide cuts one immutable version across every public module. The
release script derives module order from `go.work` and `go.mod`; do not create tags
or edit dependency versions by hand.

## Know what is released

The public release train contains `core`, `components`, `markdown`, `highlight`,
`latex`, `ptytest`, and `ssh`. Every public module receives the same version even when its
own files did not change. `examples` and `internal` are tested but never tagged.

Before v1, every exported API may change. From v1 onward, the release must preserve
Go source compatibility with the previous v1 release. The pinned `gorelease` check
enforces that boundary.

## Prepare the repository

Complete these steps on `main`:

1. Choose the next canonical `X.Y.Z` version
2. Move the entries under `Unreleased` into `## [X.Y.Z] — YYYY-MM-DD`
3. Leave a new empty `Unreleased` section for later work
4. Run the complete CI workflow and resolve every failure
5. Confirm `main` is clean and matches `origin/main`

Do not place a `replace` directive in any module. A workspace replacement is not
part of a published module and would make local verification test a different graph
from the one consumers receive.

## Inspect the dry run

Run the release command without `--execute`:

```sh
scripts/release.sh X.Y.Z
```

The dry run performs the local gate, checks first-phase API compatibility, derives
dependency phases, and prints every dependency bump and tag. It does not modify the
repository or push a tag.

Review these facts in its output:

- Every public module appears exactly once in a tag phase
- A module is tagged after every Oolong module it imports
- The proposed changelog section exists
- `gorelease` reports the expected pre-1.0 break or v1 compatibility result
- No local or remote tag already uses the version

## Execute once

After reviewing the plan, run:

```sh
scripts/release.sh X.Y.Z --execute
```

The script asks for the version again when attached to a terminal. It then updates
downstream requirements phase by phase, tests each published dependency graph,
commits and pushes dependency bumps, runs compatibility at the last safe point, and
pushes annotated module tags.

Do not interrupt the process after the first tag is pushed. A tag recorded by the Go
proxy or checksum database is immutable, even if it is later deleted from GitHub.

## Verify the result

The script verifies every remote tag and prints the commit it names. The Go proxy is
eventually consistent; query it once later instead of polling it during the release:

```sh
GOPROXY=https://proxy.golang.org GOFLAGS=-mod=mod \
  go list -m github.com/Tangerg/oolong/core@vX.Y.Z
```

Check one downstream module as well, then create GitHub release notes from the same
changelog section. Do not publish a second set of release notes with different
behavioral claims.

## Recover from a partial release

Never move, delete, or recreate a published tag. If execution stops after any tag
reaches the remote:

1. Record which module tags and dependency-bump commits were pushed
2. Fix the cause on `main`
3. Move the remaining release notes to the next patch version
4. Run a new coordinated release for every public module

The next version supersedes the partial one. If a published module version is unsafe
to select, retract it in that module's next `go.mod` release and explain the reason in
the changelog.
