package kit

// noCopy is the standard go vet marker for mutable appearance owners whose nested
// controller or frame state gives pointer identity semantic meaning. Its methods are
// never called.
type noCopy struct{}

func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}
