//go:build unix

package main

import (
	"context"
	"os/exec"
)

func editorCommand(ctx context.Context, editor, filename string) (*exec.Cmd, error) {
	// EDITOR owns shell syntax; the generated filename must remain an argument even
	// when its temporary directory contains spaces or shell metacharacters.
	//nolint:gosec // G204: EDITOR is the user's command; filename is not command source.
	return exec.CommandContext(ctx, "sh", "-c", editor+` "$@"`, "oolong-editor", filename), nil
}
