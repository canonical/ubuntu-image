package helper

import (
	"runtime/debug"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// Asserter for testing purposes
type Asserter struct {
	*testing.T
}

// AssertErrNil asserts that an error is nil. If the boolean fatal is true, it
// ends the test on a non-nil error. Otherwise, it lets the test continue running
// but marks it as failed
func (asserter *Asserter) AssertErrNil(err error, fatal bool) {
	asserter.Helper()
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
	asserter.Helper()
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

// AssertEqual asserts that two objects are equal using go-cmp
func (asserter *Asserter) AssertEqual(want, got interface{}, cmpOpts ...cmp.Option) {
	asserter.Helper()
	diff := cmp.Diff(want, got, cmpOpts...)
	if want != nil && diff != "" {
		asserter.Errorf("mismatch (-want +got):\n%s", diff)
	}
}
