//go:build unix && !linux

package mermaid_test

import "testing"

func runWithProcessReaper(*testing.T) bool { return false }

func startBrowserReaper(*testing.T, int) func() { return func() {} }
