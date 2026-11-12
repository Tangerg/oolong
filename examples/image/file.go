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
	if cause := context.Cause(ctx); cause != nil {
		return nil, cause
	}
	file, err := regularfile.Open(path)
	if err != nil {
		return nil, err
	}
	// A successful close must preserve a bare cancellation for task outcome classification.
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	tooLarge := func() error { return fmt.Errorf("image: PNG exceeds %d bytes", maxPNGFileBytes) }
	if info.Size() > maxPNGFileBytes {
		return nil, tooLarge()
	}
	data, err = io.ReadAll(contextReader{ctx, io.LimitReader(file, maxPNGFileBytes+1)})
	if err != nil {
		return nil, err
	}
	if len(data) > maxPNGFileBytes {
		return nil, tooLarge()
	}
	return data, nil
}

// Cancellation is checked around regular-file reads. It cannot interrupt a
// filesystem that blocks inside its own kernel read or open operation.
type contextReader struct {
	context context.Context //nolint:containedctx // The adapter lives within one readPNG call; io.Reader cannot accept that call's context.
	reader  io.Reader
}

func (r contextReader) Read(data []byte) (int, error) {
	if err := context.Cause(r.context); err != nil {
		return 0, err
	}
	n, err := r.reader.Read(data)
	cause := context.Cause(r.context)
	if cause == nil || errors.Is(err, cause) {
		return n, err
	}
	// A wrapped EOF may carry an independent failure that cancellation cannot erase.
	if err == nil || err == io.EOF {
		return n, cause
	}
	return n, errors.Join(err, cause)
}
