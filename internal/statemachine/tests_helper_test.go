package statemachine

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/google/uuid"
	"github.com/snapcore/snapd/osutil"

	"github.com/canonical/ubuntu-image/internal/helper"
	"github.com/canonical/ubuntu-image/internal/imagedefinition"
)

var basicImageDef = imagedefinition.ImageDefinition{
	Architecture: getHostArch(),
	Series:       getHostSuite(),
	Rootfs: &imagedefinition.Rootfs{
		Archive: "ubuntu",
	},
	Customization: &imagedefinition.Customization{},
}

// basicChroot holds the path to the basic chroot
// this variable should be treated as a singleton
var basicChroot *basicChrooter

// basicChrooter provides a way to manage a basic chroot
type basicChrooter struct {
	m    sync.Mutex
	path string
}

// NewBasicChroot creates but do not really initializes
// a basicChroot. Initialization will be done by the first
// user of the basicChroot
func NewBasicChroot() *basicChrooter {
	return &basicChrooter{
		m: sync.Mutex{},
	}
}

// isInit safely checks if the basicChroot is initialized
func (b *basicChrooter) isInit() bool {
	b.m.Lock()
	defer b.m.Unlock()
	return len(b.path) != 0
}

// init initializes the basicChroot
func (b *basicChrooter) init() error {
	// We need to protect the whole time the chroot is being created
	// because we do not want to trigger multiple chroot creation in parallel
	// This may block several tests during ~1-2min but this is an acceptable
	// drawback to then win some time.
	b.m.Lock()
	defer b.m.Unlock()

	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.ImageDef = basicImageDef
	path := filepath.Join("/tmp", "ubuntu-image-chroot-"+uuid.NewString())
	stateMachine.tempDirs.chroot = path

	err := helper.SetDefaults(&stateMachine.ImageDef)
	if err != nil {
		return err
	}

	err = stateMachine.createChroot()
	if err != nil {
		return err
	}

	b.path = path

	return nil
}

// Clean removes the chroot
// This method is expected to be called only once at the end of the
// test suite.
func (b *basicChrooter) Clean() {
	os.RemoveAll(b.path)
}

// getBasicChroot initializes and/or set the basicChroot path to
// the given stateMachine
func getBasicChroot(s StateMachine) error {
	if !basicChroot.isInit() {
		err := basicChroot.init()
		if err != nil {
			return err
		}
	}

	return provideChroot(s)
}

// provideChroot provides a copy of the basicChroot to the given stateMachine
func provideChroot(s StateMachine) error {
	basicChroot.m.Lock()
	defer basicChroot.m.Unlock()

	return osutil.CopySpecialFile(basicChroot.path, s.tempDirs.chroot)
}

type osMockConf struct {
	osutilCopySpecialFileThreshold uint
	ReadDirThreshold               uint
	RemoveThreshold                uint
	TruncateThreshold              uint
	OpenFileThreshold              uint
}

// osMock holds methods to easily mock functions from os and snapd/osutil packages
// Each method can be configured to fail after a given number of calls
// This could be improved by letting the mock functions calls the real
// functions before failing.
type osMock struct {
	conf                            *osMockConf
	beforeOsutilCopySpecialFileFail uint
	beforeReadDirFail               uint
	beforeRemoveFail                uint
	beforeTruncateFail              uint
	beforeOpenFileFail              uint
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

func (o *osMock) Remove(name string) error {
	if o.beforeRemoveFail >= o.conf.RemoveThreshold {
		return fmt.Errorf("Remove fail")
	}
	o.beforeRemoveFail++

	return nil
}

func (o *osMock) Truncate(name string, size int64) error {
	if o.beforeTruncateFail >= o.conf.TruncateThreshold {
		return fmt.Errorf("Truncate fail")
	}
	o.beforeTruncateFail++

	return nil
}

func (o *osMock) OpenFile(name string, flag int, perm os.FileMode) (*os.File, error) {
	if o.beforeOpenFileFail >= o.conf.OpenFileThreshold {
		return nil, fmt.Errorf("OpenFile fail")
	}
	o.beforeOpenFileFail++

	return &os.File{}, nil
}

func NewOSMock(conf *osMockConf) *osMock {
	return &osMock{conf: conf}
}

type mockRunCmd struct {
	cmds []*exec.Cmd
}

func NewMockRunCommand() *mockRunCmd {
	return &mockRunCmd{}
}

func (m *mockRunCmd) runCmd(cmd *exec.Cmd, debug bool) error {
	m.cmds = append(m.cmds, cmd)
	return nil
}

type mockExecCmd struct{}

func NewMockExecCommand() *mockExecCmd {
	return &mockExecCmd{}
}

func (m *mockExecCmd) Command(cmd string, args ...string) *exec.Cmd {
	// Replace the command with an echo of it
	//nolint:gosec,G204
	return exec.Command("echo", append([]string{cmd}, args...)...)
}
