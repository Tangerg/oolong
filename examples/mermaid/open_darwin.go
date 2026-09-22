package main

import (
	"context"
	"os/exec"
)

func openImage(ctx context.Context, path string) error {
	return exec.CommandContext(ctx, "/usr/bin/open", path).Run() //nolint:gosec // G204: fixed viewer command; the argument is an application-owned exported PNG path.
}
