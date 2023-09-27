package helper

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/snapcore/snapd/osutil"
)

// define some mocked versions of go package functions
func mockRemove(string) error {
	return fmt.Errorf("Test Error")
}
func mockRename(string, string) error {
	return fmt.Errorf("Test Error")
}

// TestRestoreResolvConf tests if resolv.conf is restored correctly
func TestRestoreResolvConf(t *testing.T) {
	t.Run("test_restore_resolv_conf", func(t *testing.T) {
		asserter := Asserter{T: t}
		// Prepare temporary directory
		workDir := filepath.Join("/tmp", "ubuntu-image-"+uuid.NewString())
		err := os.Mkdir(workDir, 0755)
		asserter.AssertErrNil(err, true)
		defer os.RemoveAll(workDir)

		// Create test objects for a regular backup
		err = os.MkdirAll(filepath.Join(workDir, "etc"), 0755)
		asserter.AssertErrNil(err, true)
		mainConfPath := filepath.Join(workDir, "etc", "resolv.conf")
		mainConf, err := os.Create(mainConfPath)
		asserter.AssertErrNil(err, true)
		testData := []byte("Main")
		_, err = mainConf.Write(testData)
		asserter.AssertErrNil(err, true)
		mainConf.Close()
		backupConfPath := filepath.Join(workDir, "etc", "resolv.conf.tmp")
		backupConf, err := os.Create(backupConfPath)
		asserter.AssertErrNil(err, true)
		testData = []byte("Backup")
		_, err = backupConf.Write(testData)
		asserter.AssertErrNil(err, true)
		backupConf.Close()

		err = RestoreResolvConf(workDir)
		asserter.AssertErrNil(err, true)
		if osutil.FileExists(backupConfPath) {
			t.Errorf("Backup resolv.conf.tmp has not been removed")
		}
		checkData, err := os.ReadFile(mainConfPath)
		asserter.AssertErrNil(err, true)
		if string(checkData) != "Backup" {
			t.Errorf("Main resolv.conf has not been restored")
		}

		// Now check if the symlink case also works
		_, err = os.Create(backupConfPath)
		asserter.AssertErrNil(err, true)
		err = os.Remove(mainConfPath)
		asserter.AssertErrNil(err, true)
		err = os.Symlink("resolv.conf.tmp", mainConfPath)
		asserter.AssertErrNil(err, true)

		err = RestoreResolvConf(workDir)
		asserter.AssertErrNil(err, true)
		if osutil.FileExists(backupConfPath) {
			t.Errorf("Backup resolv.conf.tmp has not been removed when dealing with as symlink")
		}
		if !osutil.IsSymlink(mainConfPath) {
			t.Errorf("Main resolv.conf should have remained a symlink, but it is not")
		}
	})
}

// TestFailedRestoreResolvConf tests all resolv.conf error cases
func TestFailedRestoreResolvConf(t *testing.T) {
	t.Run("test_failed_restore_resolv_conf", func(t *testing.T) {
		asserter := Asserter{T: t}
		// Prepare temporary directory
		workDir := filepath.Join("/tmp", "ubuntu-image-"+uuid.NewString())
		err := os.Mkdir(workDir, 0755)
		asserter.AssertErrNil(err, true)
		defer os.RemoveAll(workDir)

		// Create test environment
		err = os.MkdirAll(filepath.Join(workDir, "etc"), 0755)
		asserter.AssertErrNil(err, true)
		mainConfPath := filepath.Join(workDir, "etc", "resolv.conf")
		_, err = os.Create(mainConfPath)
		asserter.AssertErrNil(err, true)
		backupConfPath := filepath.Join(workDir, "etc", "resolv.conf.tmp")
		_, err = os.Create(backupConfPath)
		asserter.AssertErrNil(err, true)

		// Mock the os.Rename failure
		osRename = mockRename
		defer func() {
			osRename = os.Rename
		}()
		err = RestoreResolvConf(workDir)
		asserter.AssertErrContains(err, "Error moving file")

		// Mock the os.Remove failure
		err = os.Remove(mainConfPath)
		asserter.AssertErrNil(err, true)
		err = os.Symlink("resolv.conf.tmp", mainConfPath)
		asserter.AssertErrNil(err, true)
		osRemove = mockRemove
		defer func() {
			osRemove = os.Remove
		}()
		err = RestoreResolvConf(workDir)
		asserter.AssertErrContains(err, "Error removing file")
	})
}

type s1 struct {
	A string `default:"test"`
	B string
}

type s2 struct {
	A string `default:"test"`
	B bool   `default:"true"`
}

func TestSetDefaults(t *testing.T) {
	type args struct {
		needsDefaults interface{}
	}
	tests := []struct {
		name          string
		args          args
		want          interface{}
		wantErr       bool
		expectedError string
	}{
		{
			name: "set default on empty valid struct",
			args: args{
				needsDefaults: &s1{},
			},
			want: &s1{
				A: "test",
				B: "",
			},
		},
		{
			name: "set default on non-empty valid struct",
			args: args{
				needsDefaults: &s1{
					A: "non-empty",
				},
			},
			want: &s1{
				A: "non-empty",
				B: "",
			},
		},
		{
			name: "set default on empty struct with bool",
			args: args{
				needsDefaults: &s2{},
			},
			want: &s2{
				A: "test",
				B: true,
			},
		},
		{
			name: "set default on non-empty struct with bool",
			args: args{
				needsDefaults: &s2{
					B: false,
				},
			},
			want: &s2{
				A: "test",
				B: false,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			asserter := Asserter{T: t}
			err := SetDefaults(tc.args.needsDefaults)

			if len(tc.expectedError) == 0 {
				asserter.AssertErrNil(err, true)
				asserter.AssertEqual(tc.want, tc.args.needsDefaults)
			} else {
				asserter.AssertErrContains(err, tc.expectedError)
			}

		})
	}
}
