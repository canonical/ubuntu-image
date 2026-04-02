package helper

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/invopop/jsonschema"
	"github.com/pkg/xattr"
	"github.com/snapcore/snapd/osutil"
	"github.com/xeipuuv/gojsonschema"

	"github.com/canonical/ubuntu-image/internal/testhelper"
)

// define some mocked versions of go package functions
func mockMkdirAll(string, os.FileMode) error {
	return fmt.Errorf("Test error")
}
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
	workDir := filepath.Join(testhelper.DefaultTmpDir, "ubuntu-image-"+uuid.NewString())
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

	// Check if it works when resolv.conf was backed up as a symlink
	// to non-existent file like systemd runtime resolv-stub.conf
	_, err = os.Create(mainConfPath)
	asserter.AssertErrNil(err, true)
	err = os.Remove(backupConfPath)
	asserter.AssertErrNil(err, true)
	err = os.Symlink("non-existent-file", backupConfPath)
	asserter.AssertErrNil(err, true)

	err = RestoreResolvConf(workDir)
	asserter.AssertErrNil(err, true)
	if osutil.FileExists(backupConfPath) {
		t.Errorf("Backup resolv.conf.tmp has not been removed when pointing to non-existent file")
	}
	if !osutil.IsSymlink(mainConfPath) {
		t.Errorf("Main resolv.conf should have remained a symlink, but it is not")
	}
}

// TestFailedRestoreResolvConf tests all resolv.conf error cases
func TestFailedRestoreResolvConf(t *testing.T) {
	asserter := Asserter{T: t}
	// Prepare temporary directory
	workDir := filepath.Join(testhelper.DefaultTmpDir, "ubuntu-image-"+uuid.NewString())
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

// TestTarXattrs sets an xattr on a file, puts it in a tar archive,
// extracts the tar archive and ensures the xattr is still present
func TestTarXattrs(t *testing.T) {
	asserter := Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	// create a file with xattrs in a temporary directory
	xattrBytes := []byte("ui-test")
	testDir, err := os.MkdirTemp(testhelper.DefaultTmpDir, "ubuntu-image-xattr-test")
	asserter.AssertErrNil(err, true)
	extractDir, err := os.MkdirTemp(testhelper.DefaultTmpDir, "ubuntu-image-xattr-test")
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

	testDir, err := os.MkdirTemp(testhelper.DefaultTmpDir, "ubuntu-image-ping-xattr-test")
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

// Test_divertExecWithFake runs divertExecWtihFake with fake dpkg-divert (only moving file) and ensure the behaviour is the correct one.
func Test_DivertExecWithFake(t *testing.T) {
	asserter := Asserter{T: t}
	// Prepare temporary directory
	workDir := filepath.Join(testhelper.DefaultTmpDir, "ubuntu-image-"+uuid.NewString())
	err := os.Mkdir(workDir, 0755)
	asserter.AssertErrNil(err, true)

	// Create test environment
	err = os.MkdirAll(filepath.Join(workDir, "usr", "bin"), 0755)
	asserter.AssertErrNil(err, true)
	testFile := filepath.Join(workDir, "usr", "bin", "test")
	err = os.WriteFile(filepath.Join(workDir, "usr", "bin", "test"), []byte("test"), 0600)
	asserter.AssertErrNil(err, true)

	// Mock the DpkgDivert (as we cannot execute dpkg-divert)
	dpkgDivert = func(targetDir string, target string) (*exec.Cmd, *exec.Cmd) {
		//nolint:gosec,G204
		return exec.Command("mv", filepath.Join(targetDir, target), filepath.Join(targetDir, target+".dpkg-divert")),
			exec.Command("mv", filepath.Join(targetDir, target+".dpkg-divert"), filepath.Join(targetDir, target))
	}
	t.Cleanup(func() {
		dpkgDivert = DpkgDivert
	})
	divert, undivert := DivertExecWithFake(workDir, filepath.Join("usr", "bin", "test"), "replaced", true)
	err = divert()
	asserter.AssertErrNil(err, true)
	if !osutil.FileExists(testFile) {
		t.Errorf("replacement test file \"%s\" does not exist", testFile)
	}
	content, err := os.ReadFile(testFile)
	asserter.AssertErrNil(err, true)
	if string(content) != "replaced" {
		t.Errorf("replacement test file \"%s\" does not have correct content: \"%s\" != \"%s\"", testFile, string(content), "replaced")
	}
	if !osutil.FileExists(testFile + ".dpkg-divert") {
		t.Errorf("diverted test file \"%s\" does not exist", testFile+".dpkg-divert")
	}
	content, err = os.ReadFile(testFile + ".dpkg-divert")
	asserter.AssertErrNil(err, true)
	if string(content) != "test" {
		t.Errorf("diverted test file \"%s\" does not have correct content: \"%s\" != \"%s\"", testFile+".dpkg-divert", string(content), "test")
	}
	err = undivert(nil)
	asserter.AssertErrNil(err, true)
	if osutil.FileExists(testFile + ".dpkg-divert") {
		t.Errorf("diverted test file \"%s\" is still here", testFile+".dpkg-divert")
	}
	if !osutil.FileExists(testFile) {
		t.Errorf("test file \"%s\" does not exist anymore", testFile)
	}
	content, err = os.ReadFile(testFile)
	asserter.AssertErrNil(err, true)
	if string(content) != "test" {
		t.Errorf("diverted test file \"%s\" does not have correct content: \"%s\" != \"%s\"", testFile, string(content), "test")
	}
}

func Test_DivertExecWithFake_fail(t *testing.T) {
	asserter := Asserter{T: t}
	// Prepare temporary directory
	workDir := filepath.Join(testhelper.DefaultTmpDir, "ubuntu-image-"+uuid.NewString())
	err := os.Mkdir(workDir, 0755)
	asserter.AssertErrNil(err, true)

	// Create test environment
	err = os.MkdirAll(filepath.Join(workDir, "usr", "bin"), 0755)
	asserter.AssertErrNil(err, true)
	testFile := filepath.Join("/usr", "bin", "test")
	testFilePath := filepath.Join(workDir, testFile)
	err = os.WriteFile(testFilePath, []byte("test"), 0600)
	asserter.AssertErrNil(err, true)

	runCmd = func(cmd *exec.Cmd, debug bool) error {
		return fmt.Errorf("Fail to run command %s", cmd.String())
	}
	t.Cleanup(func() {
		runCmd = RunCmd
	})
	divert, _ := DivertExecWithFake(workDir, testFile, "replaced", true)
	err = divert()
	asserter.AssertErrContains(err, fmt.Sprintf("Fail to run command /usr/sbin/chroot %s dpkg-divert --local", workDir))
	runCmd = RunCmd

	// Mock the DpkgDivert (as we cannot execute dpkg-divert)
	dpkgDivert = func(targetDir string, target string) (*exec.Cmd, *exec.Cmd) {
		return execCommand("true"), execCommand("true")
	}
	t.Cleanup(func() {
		dpkgDivert = DpkgDivert
	})

	osMkdirAll = mockMkdirAll
	t.Cleanup(func() {
		osMkdirAll = os.MkdirAll
	})
	divert, _ = DivertExecWithFake(workDir, testFile, "replaced", true)
	err = divert()
	asserter.AssertErrContains(err, fmt.Sprintf("Error creating %s directory", testFile))
	osMkdirAll = os.MkdirAll

	osWriteFile = mockWriteFile
	t.Cleanup(func() {
		osWriteFile = os.WriteFile
	})
	divert, _ = DivertExecWithFake(workDir, testFile, "replaced", true)
	err = divert()
	asserter.AssertErrContains(err, fmt.Sprintf("Error writing to %s", testFile))
	osWriteFile = os.WriteFile
	dpkgDivert = DpkgDivert

	osRemove = mockRemove
	t.Cleanup(func() {
		osRemove = os.Remove
	})
	_, undivert := DivertExecWithFake(workDir, testFile, "replaced", true)
	err = undivert(nil)
	asserter.AssertErrContains(err, fmt.Sprintf("Error removing %s", testFile))
	osRemove = os.Remove

	runCmd = func(cmd *exec.Cmd, debug bool) error {
		return fmt.Errorf("Fail to run command %s", cmd.String())
	}
	t.Cleanup(func() {
		runCmd = RunCmd
	})
	_, undivert = DivertExecWithFake(workDir, testFile, "replaced", true)
	err = undivert(nil)
	asserter.AssertErrContains(err, fmt.Sprintf("Fail to run command /usr/sbin/chroot %s dpkg-divert --remove", workDir))
	runCmd = RunCmd
}

func runAndCheck(t *testing.T, fn func() error, expected *regexp.Regexp) {
	asserter := Asserter{T: t}
	stdout, restoreStdout, err := CaptureStd(&os.Stdout)
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { restoreStdout() })
	err = fn()
	asserter.AssertErrNil(err, true)
	restoreStdout()
	readStdout, err := io.ReadAll(stdout)
	asserter.AssertErrNil(err, true)
	cmd := strings.TrimSpace(string(readStdout))
	if !expected.MatchString(cmd) {
		t.Errorf("Command \"%v\" does not match \"%v\"", cmd, expected.String())
	}
}

// Test_DivertExec tests divertStartStopDaemon and divertInitctl, with and without a usr-merged setup.
func Test_DivertExec(t *testing.T) {
	type testCase struct {
		name      string
		usrMerged bool // true: symlink /sbin to /usr/sbin
		cmd       func(string, bool) (func() error, func(error) error)
		execPath  string
	}

	cases := []testCase{
		{
			name:      "StartStopDaemon_withUsrMerged",
			usrMerged: true,
			cmd:       DivertStartStopDaemon,
			execPath:  "/usr/sbin/start-stop-daemon",
		},
		{
			name:      "StartStopDaemon_withoutUsrMerged",
			usrMerged: false,
			cmd:       DivertStartStopDaemon,
			execPath:  "/sbin/start-stop-daemon",
		},
		{
			name:      "Initctl_withUsrMerged",
			usrMerged: true,
			cmd:       DivertInitctl,
			execPath:  "/usr/sbin/initctl",
		},
		{
			name:      "Initctl_withoutUsrMerged",
			usrMerged: false,
			cmd:       DivertInitctl,
			execPath:  "/sbin/initctl",
		},
		{
			name:      "PolicyRcD",
			usrMerged: true,
			cmd:       DivertPolicyRcD,
			execPath:  "/usr/sbin/policy-rc.d",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			asserter := Asserter{T: t}
			workDir := filepath.Join(testhelper.DefaultTmpDir, "ubuntu-image-"+uuid.NewString())
			err := os.MkdirAll(workDir, 0755)
			asserter.AssertErrNil(err, true)

			// Create usr/sbin for both cases
			err = os.MkdirAll(filepath.Join(workDir, "usr", "sbin"), 0755)
			asserter.AssertErrNil(err, true)

			// Setup /sbin as symlink or as a directory
			sbinPath := filepath.Join(workDir, "sbin")
			if tc.usrMerged {
				// Symlink /sbin to /usr/sbin
				err = os.Symlink(filepath.Join(workDir, "usr", "sbin"), sbinPath)
				asserter.AssertErrNil(err, true)
			} else {
				// Real directory
				err = os.MkdirAll(sbinPath, 0755)
				asserter.AssertErrNil(err, true)
			}

			// Create the fake target
			_, err = os.Create(filepath.Join(workDir, "sbin", filepath.Base(tc.execPath)))
			asserter.AssertErrNil(err, true)

			execCommand = func(cmd string, args ...string) *exec.Cmd {
				//nolint:gosec,G204
				return exec.Command("echo", append([]string{cmd}, args...)...)
			}
			t.Cleanup(func() {
				execCommand = exec.Command
			})

			divert, undivert := tc.cmd(workDir, true)

			runAndCheck(t, divert, regexp.MustCompile("^chroot "+workDir+" dpkg-divert --local .* "+tc.execPath+"$"))
			runAndCheck(t, func() error { return undivert(nil) }, regexp.MustCompile("^chroot "+workDir+" dpkg-divert --remove .* "+tc.execPath+"$"))
		})
	}
}
