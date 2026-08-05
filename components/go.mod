module github.com/Tangerg/oolong/components

go 1.26.0

require github.com/Tangerg/oolong/core v0.0.0

require (
	github.com/mattn/go-runewidth v0.0.16 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/term v0.45.0 // indirect
)

// Until the first tag there is no version to require, so the sibling checkout is
// the version. This comes out the day core is tagged; a consumer never sees it
// either way, because Go ignores a replace in a module it depends on.
replace github.com/Tangerg/oolong/core => ../core
