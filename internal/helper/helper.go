package helper

import (
	"io"
	"os"
	"testing"
)

// CaptureStdout returns an io.Reader to read what was printed, and teardown
func CaptureStdout(t *testing.T) (io.Reader, func()) {
	t.Helper()
	stdout, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatal("couldn't create stdout pipe", err)
	}
	oldStdout := os.Stdout
	os.Stdout = stdoutW
	closed := false
	return stdout, func() {
		// only teardown once
		if closed {
			return
		}
		os.Stdout = oldStdout
		stdoutW.Close()
		closed = true
	}
}

// CaptureStderr returns an io.Reader to read what was printed to stderr, and teardown
func CaptureStderr(t *testing.T) (io.Reader, func()) {
	t.Helper()
	stderr, stderrW, err := os.Pipe()
	if err != nil {
		t.Fatal("couldn't create stderr pipe", err)
	}
	oldStderr := os.Stderr
	os.Stderr = stderrW
	closed := false
	return stderr, func() {
		// only teardown once
		if closed {
			return
		}
		os.Stderr = oldStderr
		stderrW.Close()
		closed = true
	}
}
