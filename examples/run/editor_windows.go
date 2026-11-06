//go:build windows

package main

import (
	"context"
	"os/exec"
	"syscall"
)

const editorFileEnvironment = "OOLONG_EDITOR_FILE"

func editorCommand(ctx context.Context, editor, filename string) (*exec.Cmd, error) {
	const interpreter = "cmd.exe"
	cmd := exec.CommandContext(ctx, interpreter)
	// cmd.exe does not use Go's default argument unquoting. /S removes only the
	// outer quotes; /D and /V:OFF prevent startup hooks and delayed expansion from
	// rewriting the filename carried in this child's environment.
	cmd.Args = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine: interpreter + ` /D /V:OFF /S /C "` + editor + ` "%` + editorFileEnvironment + `%""`,
	}
	cmd.Env = append(cmd.Environ(), editorFileEnvironment+"="+filename)
	return cmd, nil
}
