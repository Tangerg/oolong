package main

import (
	"context"
	"errors"
	"runtime"
	"syscall"

	"golang.org/x/sys/windows"
)

func openImage(ctx context.Context, path string) error {
	if err := context.Cause(ctx); err != nil {
		return err
	}
	// Shell extensions can require a COM single-threaded apartment.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED|windows.COINIT_DISABLE_OLE1DDE); err != nil && !errors.Is(err, syscall.Errno(windows.S_FALSE)) {
		return err
	}
	defer windows.CoUninitialize()
	file, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	verb, err := windows.UTF16PtrFromString("open")
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, verb, file, nil, nil, windows.SW_SHOWNORMAL)
}
