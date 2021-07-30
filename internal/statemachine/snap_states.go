package statemachine

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/snapcore/snapd/image"
	"github.com/snapcore/snapd/snap"
)

// Prepare the image
func (stateMachine *StateMachine) prepareImage() error {
	var snapStateMachine *SnapStateMachine
	snapStateMachine = stateMachine.parent.(*SnapStateMachine)

	var imageOpts image.Options
	imageOpts.Snaps = snapStateMachine.Opts.Snaps
	imageOpts.PrepareDir = snapStateMachine.tempDirs.unpack
	imageOpts.ModelFile = snapStateMachine.Args.ModelAssertion
	if snapStateMachine.Opts.Channel != "" {
		imageOpts.Channel = snapStateMachine.Opts.Channel
	}
	if snapStateMachine.Opts.DisableConsoleConf {
		customizations := image.Customizations{ConsoleConf: "disabled"}
		imageOpts.Customizations = customizations
	}

	// plug/slot sanitization not used by snap image.Prepare, make it no-op.
	snap.SanitizePlugsSlots = func(snapInfo *snap.Info) {}

	if err := image.Prepare(&imageOpts); err != nil {
		return fmt.Errorf("Error preparing image: %s", err.Error())
	}

	// set the gadget yaml location
	snapStateMachine.yamlFilePath = filepath.Join(stateMachine.tempDirs.unpack, "gadget", "meta", "gadget.yaml")

	return nil
}

// populateSnapRootfsContents uses a NewMountedFileSystemWriter to populate the rootfs
func (stateMachine *StateMachine) populateSnapRootfsContents() error {
	/*if self.disable_console_conf:
	    # For now we just touch /var/lib/console-conf/complete to disable
	    # console-conf on core images.
	    cc_dir = os.path.join(dst, 'var', 'lib', 'console-conf')
	    os.makedirs(cc_dir, exist_ok=True)
	    Path(os.path.join(cc_dir, 'complete')).touch()
	super().populate_rootfs_contents() */

	isSeeded := false
	for _, volume := range stateMachine.gadgetInfo.Volumes {
		for _, structure := range volume.Structure {
			if structure.Role == "system-seed" {
				isSeeded = true
				stateMachine.hooksAllowed = false
			}
		}
	}

	var src, dst string
	if isSeeded {
		// For now, since we only create the system-seed partition for
		// uc20 images, we hard-code to use this path for the rootfs
		// seed population.  In the future we might want to consider
		// populating other partitions from `snap prepare-image` output
		// as well, so looking into directories like system-data/ etc.
		src = filepath.Join(stateMachine.tempDirs.unpack, "system-seed")
		dst = stateMachine.tempDirs.rootfs
	} else {
		src = filepath.Join(stateMachine.tempDirs.unpack, "image")
		dst = filepath.Join(stateMachine.tempDirs.rootfs, "system-data")
		err := osMkdirAll(filepath.Join(dst, "boot"), 0755)
		if err != nil && !os.IsExist(err) {
			return fmt.Errorf("Error creating boot dir: %s", err.Error())
		}
	}

	// recursively copy the src to dst, skipping /boot for non-seeded images
	files, err := ioutil.ReadDir(src)
	if err != nil {
		return fmt.Errorf("Error reading unpack dir: %s", err.Error())
	}
	for _, srcFile := range files {
		if !isSeeded && srcFile.Name() == "boot" {
			continue
		}
		srcFile := filepath.Join(src, srcFile.Name())
		if err := osutilCopySpecialFile(srcFile, dst); err != nil {
			return fmt.Errorf("Error copying rootfs: %s", err.Error())
		}
	}

	// TODO: disable-console-conf places the "complete" file under _writable-defaults/var/lib.... Will that work?
	if err := stateMachine.processCloudInit(); err != nil {
		return err
	}
	return nil
}
