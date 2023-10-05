package statemachine

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/snapcore/snapd/osutil"

	"github.com/canonical/ubuntu-image/internal/commands"
	"github.com/canonical/ubuntu-image/internal/helper"
)

type osMockConf struct {
	osutilCopySpecialFileThreshold uint
	ReadDirThreshold               uint
}

type osMock struct {
	conf                            *osMockConf
	beforeOsutilCopySpecialFileFail uint
	beforeReadDirFail               uint
}

func (o *osMock) CopySpecialFile(path, dest string) error {
	if o.beforeOsutilCopySpecialFileFail >= o.conf.osutilCopySpecialFileThreshold {
		return fmt.Errorf("CopySpecialFile fail")
	}
	o.beforeOsutilCopySpecialFileFail++

	return nil
}

func (o *osMock) ReadDir(name string) ([]fs.DirEntry, error) {
	if o.beforeReadDirFail >= o.conf.ReadDirThreshold {
		return nil, fmt.Errorf("ReadDir fail")
	}
	o.beforeReadDirFail++

	return []fs.DirEntry{}, nil
}

func NewOSMock(conf *osMockConf) *osMock {
	return &osMock{conf: conf}
}

func TestPack_Setup(t *testing.T) {
	t.Run("test_classic_setup", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		restoreCWD := helper.SaveCWD()
		defer restoreCWD()

		var stateMachine PackStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine

		err := stateMachine.Setup()
		asserter.AssertErrNil(err, true)
	})
}

// TestPack_validateInput_fail tests a failure in the Setup() function when validating common input
func TestPack_validateInput_fail(t *testing.T) {
	testCases := []struct {
		name   string
		until  string
		thru   string
		errMsg string
	}{
		{
			name:   "invalid_until_name",
			until:  "fake step",
			thru:   "",
			errMsg: "not a valid state name",
		},
		{
			name:   "invalid_thru_name",
			until:  "",
			thru:   "fake step",
			errMsg: "not a valid state name",
		},
		{
			name:   "both_until_and_thru",
			until:  "make_temporary_directories",
			thru:   "calculate_rootfs_size",
			errMsg: "cannot specify both --until and --thru",
		},
	}
	for _, tc := range testCases {
		t.Run("test_failed_snap_setup_"+tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			restoreCWD := helper.SaveCWD()
			defer restoreCWD()

			var stateMachine PackStateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.parent = &stateMachine
			stateMachine.stateMachineFlags.Until = tc.until
			stateMachine.stateMachineFlags.Thru = tc.thru

			err := stateMachine.Setup()
			asserter.AssertErrContains(err, tc.errMsg)
		})
	}
}

// TestPack_readMetadata_fail tests a failed metadata read by passing --resume with no previous partial state machine run
func TestPack_readMetadata_fail(t *testing.T) {
	t.Run("test_failed_read_metadata", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		restoreCWD := helper.SaveCWD()
		defer restoreCWD()

		// start a --resume with no previous SM run
		var stateMachine PackStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.stateMachineFlags.Resume = true
		stateMachine.stateMachineFlags.WorkDir = testDir

		err := stateMachine.Setup()
		asserter.AssertErrContains(err, "error reading metadata file")
		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

func TestPack_populateTemporaryDirectories(t *testing.T) {
	testCases := []struct {
		name        string
		mockFuncs   func() func()
		expectedErr string
		opts        commands.PackOpts
	}{
		{
			name: "success",
			opts: commands.PackOpts{
				RootfsDir: filepath.Join("testdata", "filesystem"),
				GadgetDir: filepath.Join("testdata", "gadget_dir"),
			},
		},
		{
			name:        "fail to read files from rootfs",
			expectedErr: "Error reading rootfs dir",
			opts: commands.PackOpts{
				RootfsDir: filepath.Join("inexistent"),
				GadgetDir: filepath.Join("testdata", "gadget_dir"),
			},
		},
		{
			name: "fail to copy files to rootfs",
			mockFuncs: func() func() {
				osutilCopySpecialFile = mockCopySpecialFile
				return func() { osutilCopySpecialFile = osutil.CopySpecialFile }
			},
			expectedErr: "Error copying rootfs",
			opts: commands.PackOpts{
				RootfsDir: filepath.Join("testdata", "filesystem"),
				GadgetDir: filepath.Join("testdata", "gadget_dir"),
			},
		},
		{
			name: "fail to create needed gadget dir",
			mockFuncs: func() func() {
				osMkdir = mockMkdir
				return func() { osMkdir = os.Mkdir }
			},
			expectedErr: "Error creating scratch/gadget directory",
			opts: commands.PackOpts{
				RootfsDir: filepath.Join("testdata", "filesystem"),
				GadgetDir: filepath.Join("testdata", "gadget_dir"),
			},
		},
		{
			name:        "fail to copy to inexistent rootfs",
			expectedErr: "Error copying rootfs",
			opts: commands.PackOpts{
				RootfsDir: filepath.Join("testdata", "filesystem"),
				GadgetDir: filepath.Join("testdata", "gadget_dir"),
			},
			mockFuncs: func() func() {
				mock := NewOSMock(
					&osMockConf{
						osutilCopySpecialFileThreshold: 1,
					},
				)

				osutilCopySpecialFile = mock.CopySpecialFile
				return func() { osutilCopySpecialFile = osutil.CopySpecialFile }
			},
		},
		{
			name:        "fail to read gadget dir",
			expectedErr: "Error reading gadget dir",
			opts: commands.PackOpts{
				RootfsDir: filepath.Join("testdata", "filesystem"),
				GadgetDir: filepath.Join("testdata", "gadget_dir"),
			},
			mockFuncs: func() func() {
				mock := NewOSMock(
					&osMockConf{
						ReadDirThreshold: 1,
					},
				)

				osReadDir = mock.ReadDir
				return func() { osReadDir = os.ReadDir }
			},
		},
		{
			name:        "fail to copy gadget destination",
			expectedErr: "Error copying gadget",
			opts: commands.PackOpts{
				RootfsDir: filepath.Join("testdata", "filesystem"),
				GadgetDir: filepath.Join("testdata", "gadget_dir"),
			},
			mockFuncs: func() func() {
				mock := NewOSMock(
					&osMockConf{
						osutilCopySpecialFileThreshold: 2,
					},
				)

				osutilCopySpecialFile = mock.CopySpecialFile
				return func() { osutilCopySpecialFile = osutil.CopySpecialFile }
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			stateMachine := &PackStateMachine{
				Opts: tc.opts,
			}
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.parent = stateMachine

			err := stateMachine.makeTemporaryDirectories()
			asserter.AssertErrNil(err, true)

			t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

			if tc.mockFuncs != nil {
				restoreMock := tc.mockFuncs()
				t.Cleanup(restoreMock)
			}

			err = stateMachine.populateTemporaryDirectories()
			if err != nil || len(tc.expectedErr) != 0 {
				asserter.AssertErrContains(err, tc.expectedErr)
			}
		})
	}
}
