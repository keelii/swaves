package supervisor

import (
	"fmt"
	"net"
	"os"
	"strconv"
)

// InheritedListener restores a net.Listener from the file descriptor passed by
// the master process. Returns (nil, nil) when the listener fd env var is absent,
// which indicates the process was not launched by a supervisor.
// Must be called from a worker process launched by Run.
func InheritedListener() (net.Listener, error) {
	fd, ok, err := EnvFD(listenerFDEnv)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	file := os.NewFile(uintptr(fd), "supervisor-listener")
	if file == nil {
		return nil, fmt.Errorf("restore listener file failed")
	}
	defer func() { _ = file.Close() }()
	ln, err := net.FileListener(file)
	if err != nil {
		return nil, fmt.Errorf("restore listener failed: %w", err)
	}
	return ln, nil
}

// SignalReady writes the ready message to the pipe fd passed by the master
// process. Must be called once the worker is ready to accept connections.
func SignalReady() error {
	fd, ok, err := EnvFD(readyFDEnv)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	file := os.NewFile(uintptr(fd), "supervisor-ready")
	if file == nil {
		return fmt.Errorf("restore ready pipe failed")
	}
	defer func() { _ = file.Close() }()
	if _, err := file.WriteString(readyMessage + "\n"); err != nil {
		return fmt.Errorf("signal worker ready failed: %w", err)
	}
	return nil
}

// EnvFD reads an fd number from an environment variable.
// Returns (fd, true, nil) when set and valid.
// Returns (0, false, nil) when the variable is absent or empty.
// Returns (0, false, err) when the value is present but invalid.
func EnvFD(name string) (int, bool, error) {
	raw, ok := os.LookupEnv(name)
	if !ok || raw == "" {
		return 0, false, nil
	}
	fd, err := strconv.Atoi(raw)
	if err != nil || fd < 0 {
		return 0, false, fmt.Errorf("invalid %s: %q", name, raw)
	}
	return fd, true, nil
}
