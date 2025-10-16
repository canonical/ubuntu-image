package statemachine

import (
	"testing"

	"github.com/canonical/ubuntu-image/internal/helper"
)

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
			name:        "First codename invalid",
			aCodename:   "foo",
			bCodename:   "jammy",
			expected:    true,
			expectedErr: "unknown series: foo",
		},
		{
			name:        "Second codename invalid",
			aCodename:   "jammy",
			bCodename:   "foo",
			expected:    true,
			expectedErr: "unknown series: foo",
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
