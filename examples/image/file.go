package main

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/Tangerg/oolong/examples/internal/regularfile"
)

const maxPNGFileBytes = 8 << 20

func readPNG(ctx context.Context, path string) (data []byte, err error) {
	if cancelled := ctx.Err(); cancelled != nil {
		return nil, cancelled
	}
	file, err := regularfile.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	tooLarge := func() error { return fmt.Errorf("image: PNG exceeds %d bytes", maxPNGFileBytes) }
	if info.Size() > maxPNGFileBytes {
		return nil, tooLarge()
	}
	data, err = io.ReadAll(contextReader{ctx, io.LimitReader(file, maxPNGFileBytes+1)})
	if cancelled := ctx.Err(); cancelled != nil {
		return nil, cancelled
	}
	if err != nil {
		return nil, err
	}
	if len(data) > maxPNGFileBytes {
		return nil, tooLarge()
	}
	return data, nil
}

// Cancellation is checked between regular-file reads. It cannot interrupt a
// filesystem that blocks inside its own kernel read or open operation.
type contextReader struct {
	context context.Context //nolint:containedctx // The adapter lives within one readPNG call; io.Reader cannot accept that call's context.
	reader  io.Reader
}

func (r contextReader) Read(data []byte) (int, error) {
	if err := r.context.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(data)
}
