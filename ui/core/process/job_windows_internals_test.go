//go:build windows

package process

import (
	"testing"

	"golang.org/x/sys/windows"
)

func TestJobObservationRejectsAnUnrelatedProcess(t *testing.T) {
	job, err := NewJob()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := job.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	})
	handle, owned, err := job.observeProcess(windows.GetCurrentProcessId())
	if handle != 0 {
		t.Cleanup(func() {
			if closeErr := windows.CloseHandle(handle); closeErr != nil {
				t.Error(closeErr)
			}
		})
	}
	if err != nil || owned || handle != 0 {
		t.Fatalf("unrelated process acquired job wait ownership: handle=%v owned=%v err=%v", handle, owned, err)
	}
}

func TestJobObservationRefusesIncompleteNotifications(t *testing.T) {
	job, err := NewJob()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := job.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	})
	if err := job.verifyObserved(1); err == nil {
		t.Fatal("a notification count different from job accounting proved completion")
	}
}
