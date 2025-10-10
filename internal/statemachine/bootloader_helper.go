package statemachine

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/snapcore/snapd/gadget"
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
