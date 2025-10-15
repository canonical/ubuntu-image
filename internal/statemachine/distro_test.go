package statemachine

import (
	"testing"

	"github.com/canonical/ubuntu-image/internal/helper"
)

// Test_releaseFromCodename unit tests the releaseFromCodename function
func Test_releaseFromCodename(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name        string
		codename    string
		expected    string
		expectedErr string
	}{
		{
			name:     "Noble LTS",
			codename: "noble",
			expected: "24.04",
		},
		{
			name:     "Oracular (non-LTS)",
			codename: "oracular",
			expected: "24.10",
		},
		{
			name:        "Unknown",
			codename:    "unknown",
			expected:    "",
			expectedErr: "unable to get the release from the codename unknown",
		},
		{
			name:     "Old release - gutsy",
			codename: "gutsy",
			expected: "7.10",
		},
		{
			name:     "Old release - gutsy",
			codename: "gutsy",
			expected: "7.10",
		},
	}

	for _, tc := range testCases {
		t.Run("test_releaseFromCodename", func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			gotRelease, err := releaseFromCodename(tc.codename)
			if gotRelease != tc.expected {
				t.Errorf("Wrong value of releaseFromCodename. Expected '%s', got '%s'", tc.expected, gotRelease)
			}
			if err != nil || len(tc.expectedErr) != 0 {
				asserter.AssertErrContains(err, tc.expectedErr)
			}
		})
	}
}

// Test_isSeriesEqualOrOlder unit tests the isSeriesEqualOrOlder function
func Test_isSeriesEqualOrOlder(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name        string
		aCodename   string
		bCodename   string
		expected    bool
		expectedErr string
	}{
		{
			name:      "2 valid and equal codenames",
			aCodename: "noble",
			bCodename: "noble",
			expected:  true,
		},
		{
			name:      "2 valid codenames, first one older",
			aCodename: "jammy",
			bCodename: "noble",
			expected:  true,
		},
		{
			name:      "2 valid codenames, second one older",
			aCodename: "noble",
			bCodename: "jammy",
			expected:  false,
		},
		{
			name:        "Invalid codename",
			aCodename:   "foo",
			bCodename:   "jammy",
			expected:    false,
			expectedErr: "unable to get the release from the codename foo",
		},
	}

	for _, tc := range testCases {
		t.Run("test_isSeriesEqualOrOlder", func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			got, err := isSeriesEqualOrOlder(tc.aCodename, tc.bCodename)
			if got != tc.expected {
				t.Errorf("Wrong value of isSeriesEqualsOrOlder. Expected '%t', got '%t'", tc.expected, got)
			}
			if err != nil || len(tc.expectedErr) != 0 {
				asserter.AssertErrContains(err, tc.expectedErr)
			}
		})
	}
}
