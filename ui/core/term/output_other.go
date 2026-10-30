//go:build !unix && !windows

package term

import "os"

type terminalOutput struct{ *os.File }

func newTerminalOutput(file *os.File) (*terminalOutput, error) { return &terminalOutput{file}, nil }
func (o *terminalOutput) active(bool) error                    { return nil }
func (o *terminalOutput) Close() error                         { return nil }
