package statemachine

import (
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
		Archive:           "ubuntu",
		SourcesListDeb822: helper.BoolPtr(false),
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
