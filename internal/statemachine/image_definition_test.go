package statemachine

import (
	"testing"
)

// TestGeneratePocketList unit tests the generatePocketList function
func TestGeneratePocketList(t *testing.T) {
	testCases := []struct {
		name            string
		imageDef        ImageDefinition
		expectedPockets []string
	}{
		{
			"release",
			ImageDefinition{
				Series: "jammy",
				Rootfs: &RootfsType{
					Pocket:     "release",
					Components: []string{"main", "universe"},
					Mirror:     "http://archive.ubuntu.com/ubuntu/",
				},
			},
			[]string{},
		},
		{
			"security",
			ImageDefinition{
				Architecture: "amd64",
				Series:       "jammy",
				Rootfs: &RootfsType{
					Pocket:     "security",
					Components: []string{"main"},
					Mirror:     "http://archive.ubuntu.com/ubuntu/",
				},
			},
			[]string{"deb http://security.ubuntu.com/ubuntu/ jammy-security main\n"},
		},
		{
			"updates",
			ImageDefinition{
				Architecture: "arm64",
				Series:       "jammy",
				Rootfs: &RootfsType{
					Pocket:     "updates",
					Components: []string{"main", "universe", "multiverse"},
					Mirror:     "http://ports.ubuntu.com/",
				},
			},
			[]string{
				"deb http://ports.ubuntu.com/ jammy-security main universe multiverse\n",
				"deb http://ports.ubuntu.com/ jammy-updates main universe multiverse\n",
			},
		},
		{
			"proposed",
			ImageDefinition{
				Architecture: "amd64",
				Series:       "jammy",
				Rootfs: &RootfsType{
					Pocket:     "proposed",
					Components: []string{"main", "universe", "multiverse", "restricted"},
					Mirror:     "http://archive.ubuntu.com/ubuntu/",
				},
			},
			[]string{
				"deb http://security.ubuntu.com/ubuntu/ jammy-security main universe multiverse restricted\n",
				"deb http://archive.ubuntu.com/ubuntu/ jammy-updates main universe multiverse restricted\n",
				"deb http://archive.ubuntu.com/ubuntu/ jammy-proposed main universe multiverse restricted\n",
			},
		},
	}
	for _, tc := range testCases {
		t.Run("test_generate_pocket_list_"+tc.name, func(t *testing.T) {
			pocketList := tc.imageDef.generatePocketList()
			for _, expectedPocket := range tc.expectedPockets {
				found := false
				for _, pocket := range pocketList {
					if pocket == expectedPocket {
						found = true
					}
				}
				if !found {
					t.Errorf("Expected %s in pockets list %s, but it was not", expectedPocket, pocketList)
				}
			}
		})
	}
}
