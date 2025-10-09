package statemachine

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/snapcore/snapd/gadget"

	"github.com/canonical/ubuntu-image/internal/arch"
	"github.com/canonical/ubuntu-image/internal/helper"
)

// handleLkBootloader handles the special "lk" bootloader case where some extra
// files need to be added to the bootfs
func (stateMachine *StateMachine) handleLkBootloader(volume *gadget.Volume) error {
	if volume.Bootloader != "lk" {
		return nil
	}
	// For the LK bootloader we need to copy boot.img and snapbootsel.bin to
	// the gadget folder so they can be used as partition content. The first
	// one comes from the kernel snap, while the second one is modified by
	// the prepare_image step to set the right core and kernel for the kernel
	// command line.
	bootDir := filepath.Join(stateMachine.tempDirs.unpack,
		"image", "boot", "lk")
	gadgetDir := filepath.Join(stateMachine.tempDirs.unpack, "gadget")
	if _, err := os.Stat(bootDir); err != nil {
		return fmt.Errorf("got lk bootloader but directory %s does not exist", bootDir)
	}
	err := osMkdir(gadgetDir, 0755)
	if err != nil && !os.IsExist(err) {
		return fmt.Errorf("Failed to create gadget dir: %s", err.Error())
	}
	files, err := osReadDir(bootDir)
	if err != nil {
		return fmt.Errorf("Error reading lk bootloader dir: %s", err.Error())
	}
	for _, lkFile := range files {
		srcFile := filepath.Join(bootDir, lkFile.Name())
		if err := osutilCopySpecialFile(srcFile, gadgetDir); err != nil {
			return fmt.Errorf("Error copying lk bootloader dir: %s", err.Error())
		}
	}
	return nil
}

// handleSecureBoot handles a special case where files need to be moved from /boot/ to
// /EFI/ubuntu/ so that SecureBoot can still be used
func (stateMachine *StateMachine) handleSecureBoot(volume *gadget.Volume, targetDir string) error {
	var bootDir, ubuntuDir string
	switch volume.Bootloader {
	case "u-boot":
		bootDir = filepath.Join(stateMachine.tempDirs.unpack,
			"image", "boot", "uboot")
		ubuntuDir = targetDir
	case "piboot":
		bootDir = filepath.Join(stateMachine.tempDirs.unpack,
			"image", "boot", "piboot")
		ubuntuDir = targetDir
	case "grub":
		bootDir = filepath.Join(stateMachine.tempDirs.unpack,
			"image", "boot", "grub")
		ubuntuDir = filepath.Join(targetDir, "EFI", "ubuntu")
	}

	if _, err := os.Stat(bootDir); err != nil {
		// this won't always exist, and that's fine
		return nil
	}

	// copy the files from bootDir to ubuntuDir
	if err := osMkdirAll(ubuntuDir, 0755); err != nil {
		return fmt.Errorf("Error creating ubuntu dir: %s", err.Error())
	}

	files, err := osReadDir(bootDir)
	if err != nil {
		return fmt.Errorf("Error reading boot dir: %s", err.Error())
	}
	for _, bootFile := range files {
		srcFile := filepath.Join(bootDir, bootFile.Name())
		dstFile := filepath.Join(ubuntuDir, bootFile.Name())
		if err := osRename(srcFile, dstFile); err != nil {
			return fmt.Errorf("Error copying boot dir: %s", err.Error())
		}
	}

	return nil
}

// setupGrub mounts the resulting image and runs update-grub
// Works under the assumption rootfsPartNum and efiPartNum are valid.
func (stateMachine *StateMachine) setupGrub(rootfsVolName string, rootfsPartNum int, efiPartNum int, architecture string) (err error) {
	// create directories in which to mount the rootfs and the boot partition
	mountDir := filepath.Join(stateMachine.tempDirs.scratch, "loopback")
	err = osMkdir(mountDir, 0755)
	if err != nil && !os.IsExist(err) {
		return fmt.Errorf("Error creating scratch/loopback/boot/efi directory: %s", err.Error())
	}

	target := grubTargetFromArch(architecture)
	if len(target) == 0 {
		return fmt.Errorf("no valid efi target for the provided architecture")
	}

	// Slice used to store all the commands that need to be run
	// to setup grub in the chroot
	// setupGrubCmds should be filled as a FIFO list
	var setupGrubCmds []*exec.Cmd
	// Slice used to store all the commands that need to be run
	// to cleanup everything after the setup of grub
	// teardownCmds should be filled as a LIFO list (so new entries should added at the start of the slice)
	var teardownCmds []*exec.Cmd

	defer func() {
		err = execTeardownCmds(teardownCmds, stateMachine.commonFlags.Debug, err)
	}()

	imgPath := filepath.Join(stateMachine.commonFlags.OutputDir, stateMachine.VolumeNames[rootfsVolName])

	loopUsed, losetupDetachCmd, err := associateLoopDevice(imgPath, stateMachine.SectorSize)
	if err != nil {
		return err
	}

	teardownCmds = append([]*exec.Cmd{losetupDetachCmd}, teardownCmds...)

	teardownPrepareCmds, err := prepareGrubMountDir(mountDir, rootfsPartNum, efiPartNum, loopUsed, stateMachine.commonFlags.Debug)
	teardownCmds = append(teardownPrepareCmds, teardownCmds...)
	if err != nil {
		return err
	}
	defer func() {
		tmpErr := helperRestoreResolvConf(mountDir)
		if tmpErr != nil {
			if err != nil {
				err = fmt.Errorf("%s after previous error: %w", tmpErr.Error(), err)
			} else {
				err = fmt.Errorf("Error restoring /etc/resolv.conf in the chroot: \"%s\"", tmpErr.Error())
			}
		}
	}()

	// set up the mountpoints
	mountPoints := []*mountPoint{
		{
			src:      "devtmpfs-build",
			basePath: mountDir,
			relpath:  "/dev",
			typ:      "devtmpfs",
		},
		{
			src:      "devpts-build",
			basePath: mountDir,
			relpath:  "/dev/pts",
			typ:      "devpts",
			opts:     []string{"nodev", "nosuid"},
		},
		{
			src:      "proc-build",
			basePath: mountDir,
			relpath:  "/proc",
			typ:      "proc",
		},
		{
			src:      "sysfs-build",
			basePath: mountDir,
			relpath:  "/sys",
			typ:      "sysfs",
		},
		{
			basePath: mountDir,
			relpath:  "/run",
			bind:     true,
		},
	}

	mountCmds, umountCmds, err := generateMountPointCmds(mountPoints, stateMachine.tempDirs.scratch)
	if err != nil {
		return err
	}

	setupGrubCmds = append(setupGrubCmds, mountCmds...)
	teardownCmds = append(umountCmds, teardownCmds...)

	teardownCmds = append([]*exec.Cmd{
		execCommand("udevadm", "settle"),
	}, teardownCmds...)

	setupGrubCmds = append(setupGrubCmds,
		// udev needed to have grub-install properly work
		aptInstallCmd(mountDir, []string{"udev"}, false),
		execCommand("chroot",
			mountDir,
			"grub-install",
			loopUsed,
			"--boot-directory=/boot",
			"--efi-directory=/boot/efi",
			fmt.Sprintf("--target=%s", target),
			"--uefi-secure-boot",
			"--no-nvram",
		),
	)

	if architecture == arch.AMD64 {
		setupGrubCmds = append(setupGrubCmds,
			execCommand("chroot",
				mountDir,
				"grub-install",
				loopUsed,
				"--target=i386-pc",
			),
		)
	}

	divert, undivert := divertOSProber(mountDir)

	setupGrubCmds = append(setupGrubCmds, divert)
	teardownCmds = append([]*exec.Cmd{undivert}, teardownCmds...)

	setupGrubCmds = append(setupGrubCmds,
		execCommand("chroot",
			mountDir,
			"update-grub",
		),
	)

	// now run all the commands
	return helper.RunCmds(setupGrubCmds, stateMachine.commonFlags.Debug)
}

// grubTargetFromArch returns the proper grub-install target given the architecture.
func grubTargetFromArch(architecture string) string {
	switch architecture {
	case arch.AMD64:
		return "x86_64-efi"
	case arch.ARM64:
		return "arm64-efi"
	case arch.ARMHF:
		return "arm-efi"
	default:
		return ""
	}
}

// prepareGrubMountDir prepares a directory to run the grub installation process
func prepareGrubMountDir(mountDir string, rootfsPartNum int, bootPartNum int, loopUsed string, debug bool) ([]*exec.Cmd, error) {
	bootDir := filepath.Join(mountDir, "boot", "efi")
	// Slice used to store all the commands that need to be run
	// first to properly prepare the chroot
	// setupGrubCmds should be filled as a FIFO list
	var prepareCmds []*exec.Cmd
	teardownCmds := []*exec.Cmd{}

	teardownCmds = append(teardownCmds, execCommand("umount", mountDir))

	prepareCmds = append(prepareCmds,
		// Try to make sure udev is not racing with losetup and briefly
		// vanishing device files. See LP: #2045586
		execCommand("udevadm", "settle"),
		// mount the rootfs partition in which to run update-grub
		//nolint:gosec,G204
		execCommand("mount",
			fmt.Sprintf("%sp%d", loopUsed, rootfsPartNum),
			mountDir,
		),
	)

	if bootPartNum > 0 {
		teardownCmds = append([]*exec.Cmd{execCommand("umount", bootDir)}, teardownCmds...)
		prepareCmds = append(prepareCmds,
			execCommand("mkdir", "-p", bootDir),
			// mount the boot partition
			//nolint:gosec,G204
			execCommand("mount",
				fmt.Sprintf("%sp%d", loopUsed, bootPartNum),
				bootDir,
			),
		)
	}

	err := helper.RunCmds(prepareCmds, debug)
	if err != nil {
		return teardownCmds, err
	}

	err = helperBackupAndCopyResolvConf(mountDir)
	if err != nil {
		return teardownCmds, fmt.Errorf("Error setting up /etc/resolv.conf in the chroot: \"%s\"", err.Error())
	}

	return teardownCmds, nil
}
