//go:build windows

package process

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	jobCreationAttributeCount = 2
	jobListAttribute          = 0x0002000d
)

// JobStartConfig borrows Args and prepared standard files until Start returns;
// callers must not mutate those arguments or close the files during the call.
// The child inherits the current environment. Path is resolved with exec.LookPath
// in the caller's working directory; Dir selects only the child's directory.
// Empty Args uses Path as argv[0]. All three standard files must be non-nil.
type JobStartConfig struct {
	Path   string
	Args   []string
	Dir    string
	Stdin  *os.File
	Stdout *os.File
	Stderr *os.File
}

// Start admits one launcher with job membership installed at creation. It copies
// only the supplied standard handles, leaving their inheritance flags unchanged.
// On success, the caller must reap the returned process and eventually Close the
// job. On failure, any partially created process is terminated and reaped here.
// A failed attempt before process creation can be retried on the same job.
func (j *Job) Start(cfg JobStartConfig) (*os.Process, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if err := j.usable(); err != nil {
		return nil, err
	}
	if j.started {
		return nil, fmt.Errorf("process: job already admitted a launcher: %w", os.ErrInvalid)
	}
	if cfg.Path == "" || cfg.Stdin == nil || cfg.Stdout == nil || cfg.Stderr == nil {
		return nil, fmt.Errorf("process: incomplete job launch: %w", os.ErrInvalid)
	}
	child, err := j.start(cfg)
	if err == nil || !j.started {
		return child, err
	}
	stopErr := windows.TerminateJobObject(j.handle, jobTerminatedExitCode)
	var waitErr error
	if child != nil {
		_, waitErr = child.Wait()
	}
	return nil, errors.Join(err, stopErr, waitErr, j.wait())
}

func (j *Job) start(cfg JobStartConfig) (_ *os.Process, err error) {
	attributes, err := windows.NewProcThreadAttributeList(jobCreationAttributeCount)
	if err != nil {
		return nil, err
	}
	defer attributes.Delete()
	var handles []windows.Handle
	defer func() {
		for _, handle := range handles {
			err = errors.Join(err, windows.CloseHandle(handle))
		}
	}()
	for _, file := range []*os.File{cfg.Stdin, cfg.Stdout, cfg.Stderr} {
		var handle windows.Handle
		current := windows.CurrentProcess()
		err = windows.DuplicateHandle(current, windows.Handle(file.Fd()), current, &handle, 0, true, windows.DUPLICATE_SAME_ACCESS)
		runtime.KeepAlive(file)
		if err != nil {
			return nil, err
		}
		handles = append(handles, handle)
	}
	if err = attributes.Update(windows.PROC_THREAD_ATTRIBUTE_HANDLE_LIST, unsafe.Pointer(&handles[0]), uintptr(len(handles))*unsafe.Sizeof(handles[0])); err != nil { //nolint:gosec // G103: the attribute container retains the inherited handles until CreateProcess returns.
		return nil, err
	}
	// Assigning a job after Start would let application code spawn outside it.
	if err = attributes.Update(jobListAttribute, unsafe.Pointer(&j.handle), unsafe.Sizeof(j.handle)); err != nil { //nolint:gosec // G103: a live job handle retained by the attribute container.
		return nil, err
	}
	startup := windows.StartupInfoEx{ProcThreadAttributeList: attributes.List()}
	startup.Cb = uint32(unsafe.Sizeof(startup))
	startup.Flags = windows.STARTF_USESTDHANDLES
	startup.StdInput, startup.StdOutput, startup.StdErr = handles[0], handles[1], handles[2]
	path, err := exec.LookPath(cfg.Path)
	if err != nil {
		return nil, err
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	executable, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	args := cfg.Args
	if len(args) == 0 {
		args = []string{cfg.Path}
	}
	commandLine, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(args))
	if err != nil {
		return nil, err
	}
	var directory *uint16
	if cfg.Dir != "" {
		if directory, err = windows.UTF16PtrFromString(cfg.Dir); err != nil {
			return nil, err
		}
	}
	var info windows.ProcessInformation
	if err = windows.CreateProcess(executable, commandLine, nil, nil, true, windows.EXTENDED_STARTUPINFO_PRESENT|windows.CREATE_NO_WINDOW, nil, directory, &startup.StartupInfo, &info); err != nil {
		return nil, err
	}
	j.started = true
	defer func() { err = errors.Join(err, windows.CloseHandle(info.Thread), windows.CloseHandle(info.Process)) }()
	return os.FindProcess(int(info.ProcessId))
}
