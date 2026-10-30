// Package ssh runs an Oolong program on an accepted SSH terminal session.
//
// It is a transport adapter, not an SSH server. Authentication, host keys,
// listening, connection limits and command routing remain with the application and
// [charm.land/ssh]. Open takes the session only after that policy has accepted it,
// and joins its byte stream and PTY window changes into the ordinary program host
// contract. The caller supplies the returned Host to program.NewScreen or
// program.NewInline, then closes it after the application has stopped.
//
// Core does not know this package exists. Programs and components therefore behave
// the same on a local terminal and over SSH, while applications that do not serve
// SSH do not acquire its dependency tree.
package ssh
