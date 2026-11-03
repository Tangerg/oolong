//go:build !unix && !windows

package main

func connectionReset(error) bool { return false }
