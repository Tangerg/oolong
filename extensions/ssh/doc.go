// Package ssh runs an Oolong program on a terminal accepted by a charm.land/ssh server.
//
// Install an [Adapter] in the vacant PTY slot with server.SetOption(adapter.Install)
// as the final PTY configuration step before Serve.
// A zero Adapter uses emulation. Supply a custom allocator with [NewAdapter]
// and [AdapterConfig]. The adapter starts collecting window changes
// when the server accepts a PTY request. A client can resize before asking for
// a shell; waiting until the shell handler to collect windows can block that request.
//
// In each shell, exec or subsystem handler, pass the original session to
// [Adapter.Bind]. Use [Terminal.Open] on the returned handle once application policy
// has accepted the session. Only then are input and terminal modes acquired.
// Supply the resulting [Host] to program.NewScreen or program.NewInline and
// close it after Run. Authentication, routing, channel lifetime and exit status
// remain with the application.
package ssh
