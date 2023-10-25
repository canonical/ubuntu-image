package statemachine

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"sync"

	"github.com/snapcore/snapd/osutil"

	"github.com/canonical/ubuntu-image/internal/helper"
	"github.com/canonical/ubuntu-image/internal/imagedefinition"
)

func init() {
	basicChroot = NewBasicChroot()
}

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

type basicChrooter struct {
	m    sync.Mutex
	path string
}

func NewBasicChroot() *basicChrooter {
	return &basicChrooter{
		m: sync.Mutex{},
	}
}

func (b *basicChrooter) isInit() bool {
	b.m.Lock()
	defer b.m.Unlock()
	return len(b.path) != 0
}

func (b *basicChrooter) init() error {
	// We need to protect the whole time the chroot is being created
	// because we do not want to trigger multiple chroot creation in parallel
	b.m.Lock()
	defer b.m.Unlock()

	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.ImageDef = basicImageDef
	// prepare the dir

	err := stateMachine.createChroot()
	if err != nil {
		return err
	}

	basicChroot.path = stateMachine.tempDirs.chroot

	return nil
}

// getBasicChroot initializes and/or set the basicChroot path to
// the given stateMachine
func getBasicChroot(s StateMachine) error {
	if basicChroot.isInit() {

		return setBasicChrootPath(s)
	}

	err := basicChroot.init()
	if err != nil {
		return err
	}

	return setBasicChrootPath(s)
}

func setBasicChrootPath(s StateMachine) error {
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

type mockExecCmd struct{}

func NewMockExecCommand() *mockExecCmd {
	return &mockExecCmd{}
}

func (m *mockExecCmd) Command(cmd string, args ...string) *exec.Cmd {
	// Replace the command with an echo of it
	//nolint:gosec,G204
	return exec.Command("echo", append([]string{cmd}, args...)...)
}
