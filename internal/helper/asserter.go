package helper

import (
	"runtime/debug"
	"strings"
	"testing"
)

// Asserter for testing purposes
type Asserter struct {
	*testing.T
}

// AssertErrNil asserts that an error is nil. If the boolean fatal is true, it
// ends the test on a non-nil error. Otherwise, it lets the test continue running
// but marks it as failed
func (asserter *Asserter) AssertErrNil(err error, fatal bool) {
	if err != nil {
		debug.PrintStack()
		if fatal {
			asserter.Fatalf("Did not expect an error, got %s", err.Error())
		}
		asserter.Errorf("Did not expect an error, got %s", err.Error())
	}
}

// AssertErrContains asserts that an error is non-nil, and that the error
// message string contains a sub-string that is passed in
func (asserter *Asserter) AssertErrContains(err error, errString string) {
	if err == nil {
		debug.PrintStack()
		asserter.Error("Expected an error, but got none")
	} else {
		if !strings.Contains(err.Error(), errString) {
			debug.PrintStack()
			asserter.Errorf("Expected error to contain \"%s\", but got \"%s\"", errString, err.Error())
		}
	}
}
