module github.com/Tangerg/oolong/examples

go 1.26.0

require (
	github.com/Tangerg/oolong/components v0.0.0
	github.com/Tangerg/oolong/core v0.0.0
	github.com/Tangerg/oolong/ptytest v0.0.0
)

require (
	github.com/mattn/go-runewidth v0.0.16 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/term v0.45.0 // indirect
)

// See the note in components/go.mod: sibling checkouts stand in for versions
// until there are versions.
replace (
	github.com/Tangerg/oolong/components => ../components
	github.com/Tangerg/oolong/core => ../core
	github.com/Tangerg/oolong/ptytest => ../ptytest
)
