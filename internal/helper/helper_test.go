package helper

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/invopop/jsonschema"
	"github.com/pkg/xattr"
	"github.com/snapcore/snapd/osutil"
	"github.com/xeipuuv/gojsonschema"

	"github.com/canonical/ubuntu-image/internal/testhelper"
)

// define some mocked versions of go package functions
func mockRemove(string) error {
	return fmt.Errorf("Test Error")
}
func mockRename(string, string) error {
	return fmt.Errorf("Test Error")
}
func mockWriteFile(name string, data []byte, perm os.FileMode) error {
	return fmt.Errorf("WriteFile Error")
}

// TestRestoreResolvConf tests if resolv.conf is restored correctly
func TestRestoreResolvConf(t *testing.T) {
	t.Parallel()
	asserter := Asserter{T: t}
	// Prepare temporary directory
	workDir := filepath.Join("/tmp", "ubuntu-image-"+uuid.NewString())
	err := os.Mkdir(workDir, 0755)
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { os.RemoveAll(workDir) })

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
}

// TestFailedRestoreResolvConf tests all resolv.conf error cases
func TestFailedRestoreResolvConf(t *testing.T) {
	asserter := Asserter{T: t}
	// Prepare temporary directory
	workDir := filepath.Join("/tmp", "ubuntu-image-"+uuid.NewString())
	err := os.Mkdir(workDir, 0755)
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { os.RemoveAll(workDir) })

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
	t.Cleanup(func() {
		osRename = os.Rename
	})
	err = RestoreResolvConf(workDir)
	asserter.AssertErrContains(err, "Error moving file")

	// Mock the os.Remove failure
	err = os.Remove(mainConfPath)
	asserter.AssertErrNil(err, true)
	err = os.Symlink("resolv.conf.tmp", mainConfPath)
	asserter.AssertErrNil(err, true)
	osRemove = mockRemove
	t.Cleanup(func() {
		osRemove = os.Remove
	})
	err = RestoreResolvConf(workDir)
	asserter.AssertErrContains(err, "Error removing file")
}

type S1 struct {
	A string `default:"test"`
	B string
	C []string `default:"1,2,3"`
	D []*S3
	E *S3
}

type S2 struct {
	A string `default:"test"`
	B *bool  `default:"true"`
	C *bool  `default:"false"`
	D *bool
}

type S3 struct {
	A string `default:"defaults3value"`
}

type S4 struct {
	A int `default:"1"`
}

type S5 struct {
	A *S4
}

type S6 struct {
	A []*S4
}

type S7 struct {
	A bool `default:"true"`
}

func TestSetDefaults(t *testing.T) {
	t.Parallel()
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
			name: "set default on empty struct",
			args: args{
				needsDefaults: &S1{},
			},
			want: &S1{
				A: "test",
				B: "",
				C: []string{"1", "2", "3"},
			},
		},
		{
			name: "set default on non-empty struct",
			args: args{
				needsDefaults: &S1{
					A: "non-empty-A-value",
					B: "non-empty-B-value",
					C: []string{"non-empty-C-value"},
					D: []*S3{
						{
							A: "non-empty-A-value",
						},
					},
					E: &S3{
						A: "non-empty-A-value",
					},
				},
			},
			want: &S1{
				A: "non-empty-A-value",
				B: "non-empty-B-value",
				C: []string{"non-empty-C-value"},
				D: []*S3{
					{
						A: "non-empty-A-value",
					},
				},
				E: &S3{
					A: "non-empty-A-value",
				},
			},
		},
		{
			name: "set default on empty struct with bool",
			args: args{
				needsDefaults: &S2{},
			},
			want: &S2{
				A: "test",
				B: BoolPtr(true),
				C: BoolPtr(false),
				D: BoolPtr(false), // even default values we do not let nil pointer
			},
		},
		{
			name: "set default on non-empty struct with bool",
			args: args{
				needsDefaults: &S2{
					B: BoolPtr(false),
					D: BoolPtr(true),
				},
			},
			want: &S2{
				A: "test",
				B: BoolPtr(false),
				C: BoolPtr(false),
				D: BoolPtr(true),
			},
		},
		{
			name: "fail to set default on struct with unsuported type",
			args: args{
				needsDefaults: &S4{},
			},
			expectedError: "not supported",
		},
		{
			name: "fail to set default on struct containing a struct with unsuported type",
			args: args{
				needsDefaults: &S5{
					A: &S4{},
				},
			},
			expectedError: "not supported",
		},
		{
			name: "fail to set default on struct containing an slice of struct with unsuported type",
			args: args{
				needsDefaults: &S6{
					A: []*S4{
						{},
					},
				},
			},
			expectedError: "not supported",
		},
		{
			name: "fail to set default on a concrete object (not a pointer)",
			args: args{
				needsDefaults: S1{},
			},
			expectedError: "The argument to SetDefaults must be a pointer",
		},
		{
			name: "fail to set default on a boolean",
			args: args{
				needsDefaults: &S7{},
			},
			expectedError: "Setting default value of a boolean not supported. Use a pointer to boolean instead",
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

// TestCheckEmptyFields unit tests the CheckEmptyFields function
func TestCheckEmptyFields(t *testing.T) {
	t.Parallel()
	type testStruct2 struct {
		A string `yaml:"a" json:"fieldA" jsonschema:"required"`
		B string `yaml:"b" json:"fieldB"`
	}

	// define the struct we will use to test
	type testStruct struct {
		A string         `yaml:"a" json:"fieldA" jsonschema:"required"`
		B string         `yaml:"b" json:"fieldB"`
		C string         `yaml:"c" json:"fieldC,omitempty"`
		D []string       `yaml:"d" json:"fieldD"`
		E *string        `yaml:"e" json:"fieldE"`
		F *testStruct2   `yaml:"f" json:"fieldF"`
		G []*testStruct2 `yaml:"g" json:"fieldG"`
	}

	// generate the schema for our testStruct
	var jsonReflector jsonschema.Reflector
	schema := jsonReflector.Reflect(&testStruct{})

	valueE := "e"

	// now run CheckEmptyFields with a variety of test data
	// to ensure the correct return values
	testCases := []struct {
		name       string
		structData testStruct
		shouldPass bool
	}{
		{
			name: "success",
			structData: testStruct{
				A: "foo",
				B: "bar",
				C: "baz",
				D: []string{"a", "b"},
				E: &valueE,
				F: &testStruct2{
					A: "a",
					B: "b",
				},
				G: []*testStruct2{
					{
						A: "a",
						B: "b",
					},
				},
			},
			shouldPass: true,
		},
		{
			name: "missing_explicitly_required",
			structData: testStruct{A: "foo", B: "bar", C: "baz",
				F: &testStruct2{
					B: "b",
				},
				G: []*testStruct2{
					{
						B: "b",
					},
				},
			},
			shouldPass: false,
		},
		{
			name:       "missing_implicitly_required",
			structData: testStruct{A: "foo", C: "baz"},
			shouldPass: false,
		},
		{
			name:       "missing_omitempty",
			structData: testStruct{A: "foo", B: "bar"},
			shouldPass: true,
		},
	}
	for i, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			asserter := Asserter{T: t}

			result := new(gojsonschema.Result)
			err := CheckEmptyFields(&testCases[i].structData, result, schema)
			asserter.AssertErrNil(err, false)
			schema.Required = append(schema.Required, "fieldA")

			// make sure validation will fail only when expected
			if tc.shouldPass && !result.Valid() {
				t.Error("CheckEmptyFields had errors when it should not have")
			}
			if !tc.shouldPass && result.Valid() {
				t.Error("CheckEmptyFields did NOT have errors when it should have")
			}
		})
	}
}

func Test_CheckEmptyFields_not_a_pointer(t *testing.T) {
	type testStruct struct {
		A string `yaml:"a" json:"fieldA" jsonschema:"required"`
	}

	var jsonReflector jsonschema.Reflector
	schema := jsonReflector.Reflect(&testStruct{})

	structData := testStruct{A: "test"}
	result := new(gojsonschema.Result)

	asserter := Asserter{T: t}
	err := CheckEmptyFields(structData, result, schema)
	asserter.AssertErrContains(err, "must be a pointer")
}

func prepareMainFileToBackup(workDir string) (string, error) {
	err := os.MkdirAll(filepath.Join(workDir, "sbin"), 0755)
	if err != nil {
		return "", err
	}
	mainTargetPath := filepath.Join(workDir, "sbin", "target")
	mainConf, err := os.Create(mainTargetPath)
	if err != nil {
		return "", err
	}
	mainContent := []byte("Main")
	_, err = mainConf.Write(mainContent)
	if err != nil {
		return "", err
	}
	mainConf.Close()

	return mainTargetPath, nil
}

func prepareBackupFile(content string, mainTargetPath string) (string, error) {
	backupPath := mainTargetPath + backupExt
	mainConf, err := os.Create(backupPath)
	if err != nil {
		return "", err
	}
	_, err = mainConf.Write([]byte(content))
	if err != nil {
		return "", err
	}
	mainConf.Close()

	return backupPath, nil
}

func TestBackupReplace(t *testing.T) {
	asserter := Asserter{T: t}
	// Prepare temporary directory
	workDir := filepath.Join("/tmp", "ubuntu-image-"+uuid.NewString())
	err := os.Mkdir(workDir, 0755)
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { os.RemoveAll(workDir) })

	// Create test environment
	mainTargetPath, err := prepareMainFileToBackup(workDir)
	asserter.AssertErrNil(err, true)
	backupContent := "Backup"

	// Test backup file exists
	backupPath, err := prepareBackupFile(backupContent, mainTargetPath)
	asserter.AssertErrNil(err, true)
	restoreFunc, err := BackupReplace(mainTargetPath, backupContent)
	asserter.AssertErrNil(err, true)
	asserter.AssertEqual(nil, restoreFunc)
	err = os.Remove(backupPath)
	asserter.AssertErrNil(err, true)

	// Mock the os.Rename failure
	osRename = mockRename
	t.Cleanup(func() {
		osRename = os.Rename
	})
	restoreFunc, err = BackupReplace(mainTargetPath, backupContent)
	asserter.AssertErrContains(err, "Error moving file")
	asserter.AssertEqual(nil, restoreFunc)
	osRename = os.Rename

	// Mock the os.WriteFile failure
	osWriteFile = mockWriteFile
	t.Cleanup(func() {
		osWriteFile = os.WriteFile
	})
	restoreFunc, err = BackupReplace(mainTargetPath, backupContent)
	asserter.AssertErrContains(err, "Error writing to")
	asserter.AssertEqual(nil, restoreFunc)
	osWriteFile = os.WriteFile
	err = os.Remove(backupPath)
	asserter.AssertErrNil(err, true)

	// Test genRestoreFile
	mainTargetPath, err = prepareMainFileToBackup(workDir)
	asserter.AssertErrNil(err, true)

	restoreFunc, err = BackupReplace(mainTargetPath, backupContent)
	asserter.AssertErrNil(err, true)

	// Test no backup file anymore
	err = os.Remove(backupPath)
	asserter.AssertErrNil(err, true)

	err = restoreFunc(nil)
	asserter.AssertErrNil(err, true)

	// Mock the os.Rename failure
	_, err = prepareBackupFile(backupContent, mainTargetPath)
	asserter.AssertErrNil(err, true)

	osRename = mockRename
	t.Cleanup(func() {
		osRename = os.Rename
	})
	err = restoreFunc(nil)
	asserter.AssertErrContains(err, "Error moving file")

	osRename = os.Rename
}

// TestTarXattrs sets an xattr on a file, puts it in a tar archive,
// extracts the tar archive and ensures the xattr is still present
func TestTarXattrs(t *testing.T) {
	asserter := Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	// create a file with xattrs in a temporary directory
	xattrBytes := []byte("ui-test")
	testDir, err := os.MkdirTemp("/tmp", "ubuntu-image-xattr-test")
	asserter.AssertErrNil(err, true)
	extractDir, err := os.MkdirTemp("/tmp", "ubuntu-image-xattr-test")
	asserter.AssertErrNil(err, true)
	testFile, err := os.CreateTemp(testDir, "test-xattrs-")
	asserter.AssertErrNil(err, true)
	testFileName := filepath.Base(testFile.Name())
	t.Cleanup(func() { os.RemoveAll(testDir) })
	t.Cleanup(func() { os.RemoveAll(extractDir) })

	err = xattr.FSet(testFile, "user.test", xattrBytes)
	asserter.AssertErrNil(err, true)

	// now run the helper tar creation and extraction functions
	tarPath := filepath.Join(testDir, "test-xattrs.tar")
	err = CreateTarArchive(testDir, tarPath, "uncompressed", false)
	asserter.AssertErrNil(err, true)

	err = ExtractTarArchive(tarPath, extractDir, false)
	asserter.AssertErrNil(err, true)

	// now read the extracted file's extended attributes
	finalXattrs, err := xattr.List(filepath.Join(extractDir, testFileName))
	asserter.AssertErrNil(err, true)

	if !reflect.DeepEqual(finalXattrs, []string{"user.test"}) {
		t.Errorf("test file \"%s\" does not have correct xattrs set", testFile.Name())
	}
}

// TestPingXattrs runs the ExtractTarArchive file on a pre-made test file that contains /bin/ping
// and ensures that the security.capability extended attribute is still present
func TestPingXattrs(t *testing.T) {
	asserter := Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	testDir, err := os.MkdirTemp("/tmp", "ubuntu-image-ping-xattr-test")
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { os.RemoveAll(testDir) })
	testFile := filepath.Join("testdata", "rootfs_tarballs", "ping.tar")

	err = ExtractTarArchive(testFile, testDir, true)
	asserter.AssertErrNil(err, true)

	binPing := filepath.Join(testDir, "bin", "ping")
	pingXattrs, err := xattr.List(binPing)
	asserter.AssertErrNil(err, true)
	if !reflect.DeepEqual(pingXattrs, []string{"security.capability"}) {
		t.Error("ping has lost the security.capability xattr after tar extraction")
	}
}
