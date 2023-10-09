package statemachine

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// TestPackStateMachine_SuccessfulRun runs through a full pack state machine run and ensures
// it is successful. It creates a .img file and ensures they are the
// correct file types it also mounts the resulting .img and ensures grub was updated
func TestPackStateMachine_SuccessfulRun(t *testing.T) {
	t.Run("test_successful_pack_run", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		restoreCWD := helper.SaveCWD()
		t.Cleanup(restoreCWD)

		// We need the output directory set for this
		outputDir, err := os.MkdirTemp("/tmp", "ubuntu-image-")
		asserter.AssertErrNil(err, true)
		t.Cleanup(func() { os.RemoveAll(outputDir) })

		gadgetDir, err := os.MkdirTemp("/tmp", "ubuntu-image-gadget-")
		asserter.AssertErrNil(err, true)
		t.Cleanup(func() { os.RemoveAll(gadgetDir) })

		rootfsDir, err := os.MkdirTemp("/tmp", "ubuntu-image-rootfs-")
		asserter.AssertErrNil(err, true)
		t.Cleanup(func() { os.RemoveAll(rootfsDir) })

		var stateMachine PackStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.commonFlags.Debug = true
		stateMachine.commonFlags.Size = "5G"
		stateMachine.commonFlags.OutputDir = outputDir

		stateMachine.Opts = commands.PackOpts{
			RootfsDir: rootfsDir,
			GadgetDir: filepath.Join(gadgetDir, "gadget"),
		}

		gadgetSource := filepath.Join("testdata", "gadget_tree")
		err = osutil.CopySpecialFile(gadgetSource, gadgetDir)
		asserter.AssertErrNil(err, true)

		err = os.Rename(filepath.Join(gadgetDir, "gadget_tree"), filepath.Join(gadgetDir, "gadget"))
		asserter.AssertErrNil(err, true)

		// also copy gadget.yaml to the root of the gadget dir
		err = osutil.CopyFile(
			filepath.Join("testdata", "gadget_dir", "gadget.yaml"),
			filepath.Join(gadgetDir, "gadget", "gadget.yaml"),
			osutil.CopyFlagDefault,
		)
		asserter.AssertErrNil(err, true)

		debootstrapCmd := execCommand("debootstrap",
			"--arch", "amd64",
			"--variant=minbase",
			"--include=grub2-common",
			"jammy",
			stateMachine.Opts.RootfsDir,
			"http://archive.ubuntu.com/ubuntu/",
		)

		debootstrapOutput := helper.SetCommandOutput(debootstrapCmd, true)

		err = debootstrapCmd.Run()
		if err != nil {
			t.Errorf("Error running debootstrap command \"%s\". Error is \"%s\". Output is: \n%s",
				debootstrapCmd.String(), err.Error(), debootstrapOutput.String())
		}
		asserter.AssertErrNil(err, true)

		err = os.Mkdir(filepath.Join(stateMachine.Opts.RootfsDir, "boot", "grub"), 0755)
		asserter.AssertErrNil(err, true)

		err = stateMachine.Setup()
		asserter.AssertErrNil(err, true)

		t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

		err = stateMachine.Run()
		asserter.AssertErrNil(err, true)

		t.Cleanup(func() {
			err = stateMachine.Teardown()
			asserter.AssertErrNil(err, true)
		})

		// make sure all the artifacts were created and are the correct file types
		artifacts := map[string]string{"pc.img": "DOS/MBR boot sector"}
		for artifact, fileType := range artifacts {
			fullPath := filepath.Join(stateMachine.commonFlags.OutputDir, artifact)
			_, err := os.Stat(fullPath)
			if err != nil {
				if os.IsNotExist(err) {
					t.Errorf("File \"%s\" should exist, but does not", fullPath)
				}
			}

			// check it is the expected file type
			fileCommand := *exec.Command("file", fullPath)
			cmdOutput, err := fileCommand.CombinedOutput()
			asserter.AssertErrNil(err, true)
			if !strings.Contains(string(cmdOutput), fileType) {
				t.Errorf("File \"%s\" is the wrong file type. Expected \"%s\" but got \"%s\"",
					fullPath, fileType, string(cmdOutput))
			}
		}

		// create a directory in which to mount the rootfs
		mountDir := filepath.Join(stateMachine.tempDirs.scratch, "loopback")
		var mountImageCmds []*exec.Cmd
		var umountImageCmds []*exec.Cmd

		t.Cleanup(func() {
			for _, teardownCmd := range umountImageCmds {
				if tmpErr := teardownCmd.Run(); tmpErr != nil {
					if err != nil {
						err = fmt.Errorf("%s after previous error: %w", tmpErr, err)
					} else {
						err = tmpErr
					}
				}
			}
		})

		// set up the loopback
		mountImageCmds = append(mountImageCmds,
			//nolint:gosec,G204
			exec.Command("losetup",
				filepath.Join("/dev", "loop99"),
				filepath.Join(stateMachine.commonFlags.OutputDir, "pc.img"),
			),
		)

		// unset the loopback
		umountImageCmds = append(umountImageCmds,
			//nolint:gosec,G204
			exec.Command("losetup", "--detach", filepath.Join("/dev", "loop99")),
		)

		mountImageCmds = append(mountImageCmds,
			//nolint:gosec,G204
			exec.Command("kpartx", "-a", filepath.Join("/dev", "loop99")),
		)

		umountImageCmds = append([]*exec.Cmd{
			//nolint:gosec,G204
			exec.Command("kpartx", "-d", filepath.Join("/dev", "loop99")),
		}, umountImageCmds...,
		)

		mountImageCmds = append(mountImageCmds,
			//nolint:gosec,G204
			exec.Command("mount", filepath.Join("/dev", "mapper", "loop99p3"), mountDir), // with this example the rootfs is partition 3 mountDir
		)

		umountImageCmds = append([]*exec.Cmd{
			//nolint:gosec,G204
			exec.Command("mount", "--make-rprivate", filepath.Join("/dev", "mapper", "loop99p3")),
			//nolint:gosec,G204
			exec.Command("umount", "--recursive", filepath.Join("/dev", "mapper", "loop99p3")),
		}, umountImageCmds...,
		)

		// set up the mountpoints
		mountPoints := []string{"/dev", "/proc", "/sys"}
		for _, mountPoint := range mountPoints {
			mountCmds, umountCmds := mountFromHost(mountDir, mountPoint)
			mountImageCmds = append(mountImageCmds, mountCmds...)
			umountImageCmds = append(umountCmds, umountImageCmds...)
		}
		// make sure to unmount the disk too
		umountImageCmds = append([]*exec.Cmd{exec.Command("umount", "--recursive", mountDir)}, umountImageCmds...)

		// now run all the commands to mount the image
		for _, cmd := range mountImageCmds {
			outPut := helper.SetCommandOutput(cmd, true)
			err := cmd.Run()
			if err != nil {
				t.Errorf("Error running command \"%s\". Error is \"%s\". Output is: \n%s",
					cmd.String(), err.Error(), outPut.String())
			}
		}

		grubCfg := filepath.Join(mountDir, "boot", "grub", "grub.cfg")
		_, err = os.Stat(grubCfg)
		if err != nil {
			if os.IsNotExist(err) {
				t.Errorf("File \"%s\" should exist, but does not", grubCfg)
			}
		}
	})
}
