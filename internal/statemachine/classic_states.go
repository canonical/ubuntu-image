package statemachine

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/snapcore/snapd/image"
	"github.com/snapcore/snapd/image/preseed"
	"github.com/snapcore/snapd/interfaces/builtin"
	"github.com/snapcore/snapd/osutil"
	"github.com/snapcore/snapd/seed/seedwriter"
	"github.com/snapcore/snapd/snap"
	"github.com/snapcore/snapd/store"

	"github.com/canonical/ubuntu-image/internal/helper"
	"github.com/canonical/ubuntu-image/internal/imagedefinition"
)

var (
	seedVersionRegex   = regexp.MustCompile(`^[a-z0-9].*`)
	localePresentRegex = regexp.MustCompile(`(?m)^LANG=|LC_[A-Z_]+=`)
)

// Build the gadget tree
func (stateMachine *StateMachine) buildGadgetTree() error {
	classicStateMachine := stateMachine.parent.(*ClassicStateMachine)

	// make the gadget directory under scratch
	gadgetDir := filepath.Join(stateMachine.tempDirs.scratch, "gadget")

	err := osMkdir(gadgetDir, 0755)
	if err != nil && !os.IsExist(err) {
		return fmt.Errorf("Error creating scratch/gadget directory: %s", err.Error())
	}

	switch classicStateMachine.ImageDef.Gadget.GadgetType {
	case "git":
		err := cloneGitRepo(classicStateMachine.ImageDef, gadgetDir)
		if err != nil {
			return fmt.Errorf("Error cloning gadget repository: \"%s\"", err.Error())
		}
	case "directory":
		gadgetTreePath := strings.TrimPrefix(classicStateMachine.ImageDef.Gadget.GadgetURL, "file://")
		if !filepath.IsAbs(gadgetTreePath) {
			gadgetTreePath = filepath.Join(stateMachine.ConfDefPath, gadgetTreePath)
		}

		// copy the source tree to the workdir
		files, err := osReadDir(gadgetTreePath)
		if err != nil {
			return fmt.Errorf("Error reading gadget tree: %s", err.Error())
		}
		for _, gadgetFile := range files {
			srcFile := filepath.Join(gadgetTreePath, gadgetFile.Name())
			if err := osutilCopySpecialFile(srcFile, gadgetDir); err != nil {
				return fmt.Errorf("Error copying gadget source: %s", err.Error())
			}
		}
	}

	// now run "make" to build the gadget tree
	makeCmd := execCommand("make")

	// if a make target was specified then add it to the command
	if classicStateMachine.ImageDef.Gadget.GadgetTarget != "" {
		makeCmd.Args = append(makeCmd.Args, classicStateMachine.ImageDef.Gadget.GadgetTarget)
	}

	// add ARCH and SERIES environment variables for making the gadget tree
	makeCmd.Env = append(makeCmd.Env, []string{
		fmt.Sprintf("ARCH=%s", classicStateMachine.ImageDef.Architecture),
		fmt.Sprintf("SERIES=%s", classicStateMachine.ImageDef.Series),
	}...)
	// add the current ENV to the command
	makeCmd.Env = append(makeCmd.Env, os.Environ()...)
	makeCmd.Dir = gadgetDir

	makeOutput := helper.SetCommandOutput(makeCmd, classicStateMachine.commonFlags.Debug)

	if err := makeCmd.Run(); err != nil {
		return fmt.Errorf("Error running \"make\" in gadget source. "+
			"Error is \"%s\". Full output below:\n%s",
			err.Error(), makeOutput.String())
	}

	return nil
}

// Prepare the gadget tree
func (stateMachine *StateMachine) prepareGadgetTree() error {
	classicStateMachine := stateMachine.parent.(*ClassicStateMachine)
	gadgetDir := filepath.Join(classicStateMachine.tempDirs.unpack, "gadget")
	err := osMkdirAll(gadgetDir, 0755)
	if err != nil && !os.IsExist(err) {
		return fmt.Errorf("Error creating unpack directory: %s", err.Error())
	}
	// recursively copy the gadget tree to unpack/gadget
	var gadgetTree string
	if classicStateMachine.ImageDef.Gadget.GadgetType == "prebuilt" {
		gadgetTree = strings.TrimPrefix(classicStateMachine.ImageDef.Gadget.GadgetURL, "file://")
		if !filepath.IsAbs(gadgetTree) {
			gadgetTree, _ = filepath.Abs(gadgetTree)
		}
	} else {
		gadgetTree = filepath.Join(classicStateMachine.tempDirs.scratch, "gadget", "install")
	}
	entries, err := osReadDir(gadgetTree)
	if err != nil {
		return fmt.Errorf("Error reading gadget tree: %s", err.Error())
	}
	for _, gadgetEntry := range entries {
		srcFile := filepath.Join(gadgetTree, gadgetEntry.Name())
		if err := osutilCopySpecialFile(srcFile, gadgetDir); err != nil {
			return fmt.Errorf("Error copying gadget tree entry: %s", err.Error())
		}
	}

	classicStateMachine.YamlFilePath = filepath.Join(gadgetDir, gadgetYamlPathInTree)

	return nil
}

// fixHostname set fresh hostname since debootstrap copies /etc/hostname from build environment
func (stateMachine *StateMachine) fixHostname() error {
	hostname := filepath.Join(stateMachine.tempDirs.chroot, "etc", "hostname")
	hostnameFile, err := osOpenFile(hostname, os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("unable to open hostname file: %w", err)
	}
	defer hostnameFile.Close()
	_, err = hostnameFile.WriteString("ubuntu\n")
	if err != nil {
		return fmt.Errorf("unable to write hostname: %w", err)
	}
	return nil
}

// Bootstrap a chroot environment to install packages in. It will eventually
// become the rootfs of the image
func (stateMachine *StateMachine) createChroot() error {
	classicStateMachine := stateMachine.parent.(*ClassicStateMachine)

	if err := osMkdir(stateMachine.tempDirs.chroot, 0755); err != nil {
		return fmt.Errorf("Failed to create chroot directory %s : %s", stateMachine.tempDirs.chroot, err.Error())
	}

	debootstrapCmd := generateDebootstrapCmd(classicStateMachine.ImageDef,
		stateMachine.tempDirs.chroot,
	)

	debootstrapOutput := helper.SetCommandOutput(debootstrapCmd, classicStateMachine.commonFlags.Debug)

	if err := debootstrapCmd.Run(); err != nil {
		return fmt.Errorf("Error running debootstrap command \"%s\". Error is \"%s\". Output is: \n%s",
			debootstrapCmd.String(), err.Error(), debootstrapOutput.String())
	}

	err := stateMachine.fixHostname()
	if err != nil {
		return err
	}

	// debootstrap also copies /etc/resolv.conf from build environment; truncate it
	// as to not leak the host files into the built image
	resolvConf := filepath.Join(stateMachine.tempDirs.chroot, "etc", "resolv.conf")
	if err = osTruncate(resolvConf, 0); err != nil {
		return fmt.Errorf("Error truncating resolv.conf: %s", err.Error())
	}

	if *classicStateMachine.ImageDef.Rootfs.SourcesListDeb822 {
		err := stateMachine.setDeb822SourcesList(classicStateMachine.ImageDef.Deb822BuildSourcesList())
		if err != nil {
			return err
		}
		return stateMachine.setLegacySourcesList(imagedefinition.LegacySourcesListComment)
	}

	return stateMachine.setLegacySourcesList(classicStateMachine.ImageDef.LegacyBuildSourcesList())
}

// add PPAs to the apt sources list
func (stateMachine *StateMachine) addExtraPPAs() (err error) {
	classicStateMachine := stateMachine.parent.(*ClassicStateMachine)

	// create /etc/apt/sources.list.d in the chroot if it doesn't already exist
	sourcesListD := filepath.Join(stateMachine.tempDirs.chroot, "etc", "apt", "sources.list.d")
	err = osMkdir(sourcesListD, 0755)
	if err != nil && !os.IsExist(err) {
		err = fmt.Errorf("Failed to create apt sources.list.d: %s", err.Error())
		return err
	}

	// now create the ppa sources.list files
	tmpGPGDir, err := osMkdirTemp("/tmp", "ubuntu-image-gpg")
	if err != nil {
		err = fmt.Errorf("Error creating temp dir for gpg imports: %s", err.Error())
		return err
	}
	defer func() {
		tmpErr := osRemoveAll(tmpGPGDir)
		if tmpErr != nil {
			if err != nil {
				err = fmt.Errorf("%s after previous error: %w", tmpErr.Error(), err)
			} else {
				err = tmpErr
			}
		}
	}()

	trustedGPGD := filepath.Join(classicStateMachine.tempDirs.chroot, "etc", "apt", "trusted.gpg.d")

	for _, ppa := range classicStateMachine.ImageDef.Customization.ExtraPPAs {
		ppaFileName, ppaFileContents := createPPAInfo(ppa,
			classicStateMachine.ImageDef.Series)

		var ppaIO *os.File
		ppaFile := filepath.Join(sourcesListD, ppaFileName)
		ppaIO, err = osOpenFile(ppaFile, os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			err = fmt.Errorf("Error creating %s: %s", ppaFile, err.Error())
			return err
		}
		defer ppaIO.Close()
		_, err = ppaIO.Write([]byte(ppaFileContents))
		if err != nil {
			err = fmt.Errorf("unable to write ppa file %s: %w", ppaFile, err)
			return err
		}

		// Import keys either from the specified fingerprint or via the Launchpad API
		/* TODO: this is the logic for deb822 sources. When other projects
		(software-properties, ubuntu-release-upgrader) are ready, update
		to this logic instead.
		keyFileName := strings.Replace(ppaFileName, ".sources", ".gpg", 1)
		*/
		keyFileName := strings.Replace(ppaFileName, ".list", ".gpg", 1)
		keyFilePath := filepath.Join(trustedGPGD, keyFileName)
		err = importPPAKeys(ppa, tmpGPGDir, keyFilePath, stateMachine.commonFlags.Debug)
		if err != nil {
			err = fmt.Errorf("Error retrieving signing key for ppa \"%s\": %s",
				ppa.PPAName, err.Error())
			return err
		}
	}
	if err = osRemoveAll(tmpGPGDir); err != nil {
		err = fmt.Errorf("Error removing temporary gpg directory \"%s\": %s", tmpGPGDir, err.Error())
		return err
	}

	return nil
}

// cleanExtraPPAs cleans previously added PPA to the source list
func (stateMachine *StateMachine) cleanExtraPPAs() (err error) {
	classicStateMachine := stateMachine.parent.(*ClassicStateMachine)

	sourcesListD := filepath.Join(stateMachine.tempDirs.chroot, "etc", "apt", "sources.list.d")

	for _, ppa := range classicStateMachine.ImageDef.Customization.ExtraPPAs {
		if ppa.KeepEnabled == nil {
			return imagedefinition.ErrKeepEnabledNil
		}

		if *ppa.KeepEnabled {
			continue
		}

		ppaFileName, _ := createPPAInfo(ppa, classicStateMachine.ImageDef.Series)

		ppaFile := filepath.Join(sourcesListD, ppaFileName)
		err = osRemove(ppaFile)
		if err != nil {
			err = fmt.Errorf("Error removing %s: %s", ppaFile, err.Error())
			return err
		}

		keyFileName := strings.Replace(ppaFileName, ".list", ".gpg", 1)
		keyFilePath := filepath.Join(classicStateMachine.tempDirs.chroot,
			"etc", "apt", "trusted.gpg.d", keyFileName)
		err = osRemove(keyFilePath)
		if err != nil {
			err = fmt.Errorf("Error removing %s: %s", keyFilePath, err.Error())
			return err
		}
	}

	return nil
}

// Install packages in the chroot environment. This is accomplished by
// running commands to do the following:
// 1. Mount /proc /sys /dev and /run in the chroot
// 2. Run `apt update` in the chroot
// 3. Run `apt install <package list>` in the chroot
// 4. Unmount /proc /sys /dev and /run
func (stateMachine *StateMachine) installPackages() error {
	classicStateMachine := stateMachine.parent.(*ClassicStateMachine)

	err := helperBackupAndCopyResolvConf(classicStateMachine.tempDirs.chroot)
	if err != nil {
		return fmt.Errorf("Error setting up /etc/resolv.conf in the chroot: \"%s\"", err.Error())
	}

	// if any extra packages are specified, install them alongside the seeded packages
	if classicStateMachine.ImageDef.Customization != nil {
		for _, packageInfo := range classicStateMachine.ImageDef.Customization.ExtraPackages {
			classicStateMachine.Packages = append(classicStateMachine.Packages,
				packageInfo.PackageName)
		}
	}

	// Make sure to install the extra kernel if it is specified
	if classicStateMachine.ImageDef.Kernel != "" {
		classicStateMachine.Packages = append(classicStateMachine.Packages,
			classicStateMachine.ImageDef.Kernel)
	}

	// installPackagesCmds should be filled as a FIFO list
	var installPackagesCmds []*exec.Cmd

	// teardownCmds should be filled as a LIFO list
	var teardownCmds []*exec.Cmd

	// Make sure we left the system as clean as possible if something has gone wrong
	defer func() {
		err = execTeardown(teardownCmds, stateMachine.commonFlags.Debug, err)
	}()

	// mount some necessary partitions in the chroot
	mountPoints := []mountPoint{
		{
			relpath: "/dev",
			typ:     "devtmpfs",
			src:     "devtmpfs-build",
		},
		{
			relpath: "/dev/pts",
			typ:     "devpts",
			src:     "devpts-build",
			options: []string{"nodev", "nosuid"},
		},
		{
			relpath: "/proc",
			typ:     "proc",
			src:     "proc-build",
		},
		{
			relpath: "/sys",
			typ:     "sysfs",
			src:     "sysfs-build",
		},
		{
			relpath: "/run",
			bind:    true,
		},
	}

	for _, mp := range mountPoints {
		var mountCmds, umountCmds []*exec.Cmd
		var err error
		if mp.bind {
			mp.src, err = osMkdirTemp(stateMachine.tempDirs.scratch, strings.Trim(mp.relpath, "/"))
			if err != nil {
				return fmt.Errorf("Error mounting temporary directory for mountpoint \"%s\": \"%s\"",
					mp.relpath,
					err.Error(),
				)
			}
		}

		mountCmds, umountCmds, err = getMountCmd(mp.typ, mp.src, stateMachine.tempDirs.chroot, mp.relpath, mp.bind, mp.options...)
		if err != nil {
			return fmt.Errorf("Error preparing mountpoint \"%s\": \"%s\"",
				mp.relpath,
				err.Error(),
			)
		}

		installPackagesCmds = append(installPackagesCmds, mountCmds...)
		teardownCmds = append(umountCmds, teardownCmds...)
	}
	teardownCmds = append([]*exec.Cmd{
		execCommand("udevadm", "settle"),
	}, teardownCmds...)

	aptCmds := generateAptCmds(stateMachine.tempDirs.chroot, classicStateMachine.Packages)
	installPackagesCmds = append(installPackagesCmds, aptCmds...)

	for _, cmd := range installPackagesCmds {
		cmdOutput := helper.SetCommandOutput(cmd, classicStateMachine.commonFlags.Debug)
		err := cmd.Run()
		if err != nil {
			return fmt.Errorf("Error running command \"%s\". Error is \"%s\". Output is: \n%s",
				cmd.String(), err.Error(), cmdOutput.String())
		}
	}

	return nil
}

// Verify artifact names have volumes listed for multi-volume gadgets and set
// the volume names in the struct
func (stateMachine *StateMachine) verifyArtifactNames() error {
	classicStateMachine := stateMachine.parent.(*ClassicStateMachine)

	stateMachine.VolumeNames = make(map[string]string)

	if len(stateMachine.GadgetInfo.Volumes) > 1 {
		// first handle .img files if they are specified
		if classicStateMachine.ImageDef.Artifacts.Img != nil {
			for _, img := range *classicStateMachine.ImageDef.Artifacts.Img {
				if img.ImgVolume == "" {
					return fmt.Errorf("Volume names must be specified for each image when using a gadget with more than one volume")
				}
				stateMachine.VolumeNames[img.ImgVolume] = img.ImgName
			}
		}
		// qcow2 img logic is more complicated. If .img artifacts are already specified
		// in the image definition for corresponding volumes, we will re-use those and
		// convert them to a qcow2 image. Otherwise, we will create a raw .img file to
		// use as an input file for the conversion.
		// The names of these images are placed in the VolumeNames map, which is used
		// as an input file for an eventual `qemu-convert` operation.
		if classicStateMachine.ImageDef.Artifacts.Qcow2 != nil {
			for _, qcow2 := range *classicStateMachine.ImageDef.Artifacts.Qcow2 {
				if qcow2.Qcow2Volume == "" {
					return fmt.Errorf("Volume names must be specified for each image when using a gadget with more than one volume")
				}
				// We can save a whole lot of disk I/O here if the volume is
				// already specified as a .img file
				if classicStateMachine.ImageDef.Artifacts.Img != nil {
					found := false
					for _, img := range *classicStateMachine.ImageDef.Artifacts.Img {
						if img.ImgVolume == qcow2.Qcow2Volume {
							found = true
						}
					}
					if !found {
						// if a .img artifact for this volume isn't explicitly stated in
						// the image definition, then create one
						stateMachine.VolumeNames[qcow2.Qcow2Volume] = fmt.Sprintf("%s.img", qcow2.Qcow2Name)
					}
				} else {
					// no .img artifacts exist in the image definition,
					// but we still need to create one to convert to qcow2
					stateMachine.VolumeNames[qcow2.Qcow2Volume] = fmt.Sprintf("%s.img", qcow2.Qcow2Name)
				}
			}
		}
	} else {
		if classicStateMachine.ImageDef.Artifacts.Img != nil {
			img := (*classicStateMachine.ImageDef.Artifacts.Img)[0]
			if img.ImgVolume == "" {
				// there is only one volume, so get it from the map
				volName := reflect.ValueOf(stateMachine.GadgetInfo.Volumes).MapKeys()[0].String()
				stateMachine.VolumeNames[volName] = img.ImgName
			} else {
				stateMachine.VolumeNames[img.ImgVolume] = img.ImgName
			}
		}
		// qcow2 img logic is more complicated. If .img artifacts are already specified
		// in the image definition for corresponding volumes, we will re-use those and
		// convert them to a qcow2 image. Otherwise, we will create a raw .img file to
		// use as an input file for the conversion.
		// The names of these images are placed in the VolumeNames map, which is used
		// as an input file for an eventual `qemu-convert` operation.
		if classicStateMachine.ImageDef.Artifacts.Qcow2 != nil {
			qcow2 := (*classicStateMachine.ImageDef.Artifacts.Qcow2)[0]
			if qcow2.Qcow2Volume == "" {
				volName := reflect.ValueOf(stateMachine.GadgetInfo.Volumes).MapKeys()[0].String()
				if classicStateMachine.ImageDef.Artifacts.Img != nil {
					qcow2.Qcow2Volume = volName
					(*classicStateMachine.ImageDef.Artifacts.Qcow2)[0] = qcow2
					return nil // We will re-use the .img file in this case
				}
				// there is only one volume, so get it from the map
				stateMachine.VolumeNames[volName] = fmt.Sprintf("%s.img", qcow2.Qcow2Name)
				qcow2.Qcow2Volume = volName
				(*classicStateMachine.ImageDef.Artifacts.Qcow2)[0] = qcow2
			} else {
				if classicStateMachine.ImageDef.Artifacts.Img != nil {
					return nil // We will re-use the .img file in this case
				}
				stateMachine.VolumeNames[qcow2.Qcow2Volume] = fmt.Sprintf("%s.img", qcow2.Qcow2Name)
			}
		}
	}
	return nil
}

// Build a rootfs from a list of archive tasks
func (stateMachine *StateMachine) buildRootfsFromTasks() error {
	// currently a no-op pending implementation of the classic image redesign
	return nil
}

// Extract the rootfs from a tar archive
func (stateMachine *StateMachine) extractRootfsTar() error {
	classicStateMachine := stateMachine.parent.(*ClassicStateMachine)

	// make the chroot directory to which we will extract the tar
	if err := osMkdir(stateMachine.tempDirs.chroot, 0755); err != nil {
		return fmt.Errorf("Failed to create chroot directory: %s", err.Error())
	}

	// convert the URL to a file path
	// no need to check error here as the validity of the URL
	// has been confirmed by the schema validation
	tarPath := strings.TrimPrefix(classicStateMachine.ImageDef.Rootfs.Tarball.TarballURL, "file://")
	if !filepath.IsAbs(tarPath) {
		tarPath = filepath.Join(stateMachine.ConfDefPath, tarPath)
	}

	// if the sha256 sum of the tarball is provided, make sure it matches
	if classicStateMachine.ImageDef.Rootfs.Tarball.SHA256sum != "" {
		tarSHA256, err := helper.CalculateSHA256(tarPath)
		if err != nil {
			return err
		}
		if tarSHA256 != classicStateMachine.ImageDef.Rootfs.Tarball.SHA256sum {
			return fmt.Errorf("Calculated SHA256 sum of rootfs tarball \"%s\" does not match "+
				"the expected value specified in the image definition: \"%s\"",
				tarSHA256, classicStateMachine.ImageDef.Rootfs.Tarball.SHA256sum)
		}
	}

	// now extract the archive
	return helper.ExtractTarArchive(tarPath, stateMachine.tempDirs.chroot,
		stateMachine.commonFlags.Verbose, stateMachine.commonFlags.Debug)
}

// germinate runs the germinate binary and parses the output to create
// a list of packages from the seed section of the image definition
func (stateMachine *StateMachine) germinate() error {
	classicStateMachine := stateMachine.parent.(*ClassicStateMachine)

	// create a scratch directory to run germinate in
	germinateDir := filepath.Join(classicStateMachine.stateMachineFlags.WorkDir, "germinate")
	err := osMkdir(germinateDir, 0755)
	if err != nil && !os.IsExist(err) {
		return fmt.Errorf("Error creating germinate directory: \"%s\"", err.Error())
	}

	germinateCmd := generateGerminateCmd(classicStateMachine.ImageDef)
	germinateCmd.Dir = germinateDir

	germinateOutput := helper.SetCommandOutput(germinateCmd, classicStateMachine.commonFlags.Debug)

	if err := germinateCmd.Run(); err != nil {
		return fmt.Errorf("Error running germinate command \"%s\". Error is \"%s\". Output is: \n%s",
			germinateCmd.String(), err.Error(), germinateOutput.String())
	}

	packageMap := make(map[string]*[]string)
	packageMap[".seed"] = &classicStateMachine.Packages
	packageMap[".snaps"] = &classicStateMachine.Snaps
	for fileExtension, packageList := range packageMap {
		for _, fileName := range classicStateMachine.ImageDef.Rootfs.Seed.Names {
			seedFilePath := filepath.Join(germinateDir, fileName+fileExtension)
			seedFile, err := osOpen(seedFilePath)
			if err != nil {
				return fmt.Errorf("Error opening seed file %s: \"%s\"", seedFilePath, err.Error())
			}
			defer seedFile.Close()

			seedScanner := bufio.NewScanner(seedFile)
			for seedScanner.Scan() {
				seedLine := seedScanner.Bytes()
				if seedVersionRegex.Match(seedLine) {
					packageName := strings.Split(string(seedLine), " ")[0]
					*packageList = append(*packageList, packageName)
				}
			}
		}
	}

	return nil
}

// customizeCloudInitFile customizes a cloud-init data file with the given content
func customizeCloudInitFile(customData string, seedPath string, fileName string, requireHeader bool) error {
	if customData == "" {
		return nil
	}
	f, err := osCreate(path.Join(seedPath, fileName))
	if err != nil {
		return err
	}
	defer f.Close()

	if requireHeader && !strings.HasPrefix(customData, "#cloud-config\n") {
		return fmt.Errorf("provided cloud-init customization for %s is missing proper header", fileName)
	}

	_, err = f.WriteString(customData)
	if err != nil {
		return err
	}

	return nil
}

// Customize Cloud init with the values in the image definition YAML
func (stateMachine *StateMachine) customizeCloudInit() error {
	classicStateMachine := stateMachine.parent.(*ClassicStateMachine)

	cloudInitCustomization := classicStateMachine.ImageDef.Customization.CloudInit

	seedPath := path.Join(classicStateMachine.tempDirs.chroot, "var/lib/cloud/seed/nocloud")
	err := osMkdirAll(seedPath, 0755)
	if err != nil {
		return err
	}

	err = customizeCloudInitFile(cloudInitCustomization.MetaData, seedPath, "meta-data", false)
	if err != nil {
		return err
	}

	err = customizeCloudInitFile(cloudInitCustomization.UserData, seedPath, "user-data", true)
	if err != nil {
		return err
	}

	err = customizeCloudInitFile(cloudInitCustomization.NetworkConfig, seedPath, "network-config", false)
	if err != nil {
		return err
	}

	datasourceConfig := "# to update this file, run dpkg-reconfigure cloud-init\ndatasource_list: [ NoCloud ]\n"

	dpkgConfigPath := path.Join(classicStateMachine.tempDirs.chroot, "etc/cloud/cloud.cfg.d/90_dpkg.cfg")
	dpkgConfigFile, err := osCreate(dpkgConfigPath)
	if err != nil {
		return err
	}
	defer dpkgConfigFile.Close()

	_, err = dpkgConfigFile.WriteString(datasourceConfig)

	return err
}

// Customize /etc/fstab based on values in the image definition
func (stateMachine *StateMachine) customizeFstab() error {
	classicStateMachine := stateMachine.parent.(*ClassicStateMachine)

	fstabPath := filepath.Join(stateMachine.tempDirs.chroot, "etc", "fstab")

	fstabIO, err := osOpenFile(fstabPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("Error opening fstab: %s", err.Error())
	}
	defer fstabIO.Close()

	var fstabEntries []string
	for _, fstab := range classicStateMachine.ImageDef.Customization.Fstab {
		var dumpString string
		if fstab.Dump {
			dumpString = "1"
		} else {
			dumpString = "0"
		}
		fstabEntry := fmt.Sprintf("LABEL=%s\t%s\t%s\t%s\t%s\t%d",
			fstab.Label,
			fstab.Mountpoint,
			fstab.FSType,
			fstab.MountOptions,
			dumpString,
			fstab.FsckOrder,
		)
		fstabEntries = append(fstabEntries, fstabEntry)
	}

	_, err = fstabIO.Write([]byte(strings.Join(fstabEntries, "\n") + "\n"))

	return err
}

// Handle any manual customizations specified in the image definition
func (stateMachine *StateMachine) manualCustomization() error {
	classicStateMachine := stateMachine.parent.(*ClassicStateMachine)

	// copy /etc/resolv.conf from the host system into the chroot if it hasn't already been done
	err := helperBackupAndCopyResolvConf(classicStateMachine.tempDirs.chroot)
	if err != nil {
		return fmt.Errorf("Error setting up /etc/resolv.conf in the chroot: \"%s\"", err.Error())
	}

	err = manualMakeDirs(classicStateMachine.ImageDef.Customization.Manual.MakeDirs, stateMachine.tempDirs.chroot, stateMachine.commonFlags.Debug)
	if err != nil {
		return err
	}

	err = manualCopyFile(classicStateMachine.ImageDef.Customization.Manual.CopyFile, classicStateMachine.ConfDefPath, stateMachine.tempDirs.chroot, stateMachine.commonFlags.Debug)
	if err != nil {
		return err
	}

	err = manualExecute(classicStateMachine.ImageDef.Customization.Manual.Execute, stateMachine.tempDirs.chroot, stateMachine.commonFlags.Debug)
	if err != nil {
		return err
	}

	err = manualTouchFile(classicStateMachine.ImageDef.Customization.Manual.TouchFile, stateMachine.tempDirs.chroot, stateMachine.commonFlags.Debug)
	if err != nil {
		return err
	}

	err = manualAddGroup(classicStateMachine.ImageDef.Customization.Manual.AddGroup, stateMachine.tempDirs.chroot, stateMachine.commonFlags.Debug)
	if err != nil {
		return err
	}

	err = manualAddUser(classicStateMachine.ImageDef.Customization.Manual.AddUser, stateMachine.tempDirs.chroot, stateMachine.commonFlags.Debug)
	if err != nil {
		return err
	}

	return nil
}

// prepareClassicImage calls image.Prepare to stage snaps in classic images
func (stateMachine *StateMachine) prepareClassicImage() error {
	classicStateMachine := stateMachine.parent.(*ClassicStateMachine)

	var imageOpts image.Options

	var err error
	imageOpts.Snaps, imageOpts.SnapChannels, err = parseSnapsAndChannels(classicStateMachine.Snaps)
	if err != nil {
		return err
	}
	if stateMachine.commonFlags.Channel != "" {
		imageOpts.Channel = stateMachine.commonFlags.Channel
	}

	// plug/slot sanitization needed by provider handling
	snap.SanitizePlugsSlots = builtin.SanitizePlugsSlots

	// check if the rootfs is already preseeded. This can happen when building from a
	// rootfs tarball
	if osutil.FileExists(filepath.Join(stateMachine.tempDirs.chroot, "var", "lib", "snapd", "state.json")) {
		// first get a list of all preseeded snaps
		// seededSnaps maps the snap name and channel that was seeded
		preseededSnaps, err := getPreseededSnaps(classicStateMachine.tempDirs.chroot)
		if err != nil {
			return fmt.Errorf("Error getting list of preseeded snaps from existing rootfs: %s",
				err.Error())
		}
		for snap, channel := range preseededSnaps {
			// if a channel is specified on the command line for a snap that was already
			// preseeded, use the channel from the command line instead of the channel
			// that was originally used for the preseeding
			if !helper.SliceHasElement(imageOpts.Snaps, snap) {
				imageOpts.Snaps = append(imageOpts.Snaps, snap)
				imageOpts.SnapChannels[snap] = channel
			}
		}
		// preseed.ClassicReset automatically has some output that we only want for
		// verbose or greater logging
		if !stateMachine.commonFlags.Debug && !stateMachine.commonFlags.Verbose {
			oldPreseedStdout := preseed.Stdout
			preseed.Stdout = io.Discard
			defer func() {
				preseed.Stdout = oldPreseedStdout
			}()
		}
		// We need to use the snap-preseed binary for the reset as well, as using
		// preseed.ClassicReset() might leave us in a chroot jail
		cmd := execCommand("/usr/lib/snapd/snap-preseed", "--reset", stateMachine.tempDirs.chroot)
		err = cmd.Run()
		if err != nil {
			return fmt.Errorf("Error resetting preseeding in the chroot. Error is \"%s\"", err.Error())
		}
	}

	// iterate through the list of snaps and ensure that all of their bases
	// are also set to be installed. Note we only do this for snaps that are
	// seeded. Users are expected to specify all base and content provider
	// snaps in the image definition.
	snapStore := store.New(nil, nil)
	snapContext := context.Background()
	for _, seededSnap := range imageOpts.Snaps {
		snapSpec := store.SnapSpec{Name: seededSnap}
		snapInfo, err := snapStore.SnapInfo(snapContext, snapSpec, nil)
		if err != nil {
			return fmt.Errorf("Error getting info for snap %s: \"%s\"",
				seededSnap, err.Error())
		}
		if snapInfo.Base != "" && !helper.SliceHasElement(imageOpts.Snaps, snapInfo.Base) {
			imageOpts.Snaps = append(imageOpts.Snaps, snapInfo.Base)
		}
	}

	// add any extra snaps from the image definition to the list
	// this is done last to ensure the correct channels are being used
	if classicStateMachine.ImageDef.Customization != nil && len(classicStateMachine.ImageDef.Customization.ExtraSnaps) > 0 {
		imageOpts.SeedManifest = seedwriter.NewManifest()
		for _, extraSnap := range classicStateMachine.ImageDef.Customization.ExtraSnaps {
			if !helper.SliceHasElement(imageOpts.Snaps, extraSnap.SnapName) {
				imageOpts.Snaps = append(imageOpts.Snaps, extraSnap.SnapName)
			}
			if extraSnap.Channel != "" {
				imageOpts.SnapChannels[extraSnap.SnapName] = extraSnap.Channel
			}
			if extraSnap.SnapRevision != 0 {
				fmt.Printf("WARNING: revision %d for snap %s may not be the latest available version!\n",
					extraSnap.SnapRevision,
					extraSnap.SnapName,
				)
				err = imageOpts.SeedManifest.SetAllowedSnapRevision(extraSnap.SnapName, snap.R(extraSnap.SnapRevision))
				if err != nil {
					return fmt.Errorf("error dealing with the extra snap %s: %w", extraSnap.SnapName, err)
				}
			}
		}
	}

	modelAssertionPath := strings.TrimPrefix(classicStateMachine.ImageDef.ModelAssertion, "file://")
	// if no explicit model assertion was given, keep empty ModelFile to let snapd fallback to default
	// model assertion
	if len(modelAssertionPath) != 0 {
		if !filepath.IsAbs(modelAssertionPath) {
			imageOpts.ModelFile = filepath.Join(stateMachine.ConfDefPath, modelAssertionPath)
		} else {
			imageOpts.ModelFile = modelAssertionPath
		}
	}

	imageOpts.Classic = true
	imageOpts.Architecture = classicStateMachine.ImageDef.Architecture
	imageOpts.PrepareDir = classicStateMachine.tempDirs.chroot
	imageOpts.Customizations = *new(image.Customizations)
	imageOpts.Customizations.Validation = stateMachine.commonFlags.Validation

	// image.Prepare automatically has some output that we only want for
	// verbose or greater logging
	if !stateMachine.commonFlags.Debug && !stateMachine.commonFlags.Verbose {
		oldImageStdout := image.Stdout
		image.Stdout = io.Discard
		defer func() {
			image.Stdout = oldImageStdout
		}()
	}

	if err := imagePrepare(&imageOpts); err != nil {
		return fmt.Errorf("Error preparing image: %s", err.Error())
	}

	return nil
}

// preseedClassicImage preseeds the snaps that have already been staged in the chroot
func (stateMachine *StateMachine) preseedClassicImage() (err error) {
	classicStateMachine := stateMachine.parent.(*ClassicStateMachine)

	// preseedCmds should be filled as a FIFO list
	var preseedCmds []*exec.Cmd
	// teardownCmds should be filled as a LIFO list to unmount first what was mounted last
	var teardownCmds []*exec.Cmd

	// set up the mount commands
	mountPoints := []mountPoint{
		{
			relpath: "/dev",
			typ:     "devtmpfs",
			src:     "devtmpfs-build",
		},
		{
			relpath: "/dev/pts",
			typ:     "devpts",
			src:     "devpts-build",
			options: []string{"nodev", "nosuid"},
		},
		{
			relpath: "/proc",
			typ:     "proc",
			src:     "proc-build",
		},
		{
			relpath: "/sys/kernel/security",
			typ:     "securityfs",
			src:     "none",
		},
		{
			relpath: "/sys/fs/cgroup",
			typ:     "cgroup2",
			src:     "none",
		},
	}

	// Make sure we left the system as clean as possible if something has gone wrong
	defer func() {
		err = execTeardown(teardownCmds, stateMachine.commonFlags.Debug, err)
	}()

	for _, mountPoint := range mountPoints {
		mountCmds, umountCmds, err := getMountCmd(mountPoint.typ, mountPoint.src, stateMachine.tempDirs.chroot, mountPoint.relpath, mountPoint.bind, mountPoint.options...)
		if err != nil {
			return fmt.Errorf("Error preparing mountpoint \"%s\": \"%s\"",
				mountPoint.relpath,
				err.Error(),
			)
		}
		preseedCmds = append(preseedCmds, mountCmds...)
		teardownCmds = append(umountCmds, teardownCmds...)
	}

	preseedCmds = append(preseedCmds,
		//nolint:gosec,G204
		exec.Command("/usr/lib/snapd/snap-preseed", stateMachine.tempDirs.chroot),
	)
	for _, cmd := range preseedCmds {
		cmdOutput := helper.SetCommandOutput(cmd, classicStateMachine.commonFlags.Debug)
		err := cmd.Run()
		if err != nil {
			return fmt.Errorf("Error running command \"%s\". Error is \"%s\". Output is: \n%s",
				cmd.String(), err.Error(), cmdOutput.String())
		}
	}
	return nil
}

// populateClassicRootfsContents copies over the staged rootfs
// to rootfs. It also changes fstab and handles the --cloud-init flag
func (stateMachine *StateMachine) populateClassicRootfsContents() error {
	classicStateMachine := stateMachine.parent.(*ClassicStateMachine)

	// if we backed up resolv.conf then restore it here
	err := helperRestoreResolvConf(classicStateMachine.tempDirs.chroot)
	if err != nil {
		return fmt.Errorf("Error restoring /etc/resolv.conf in the chroot: \"%s\"", err.Error())
	}

	files, err := osReadDir(stateMachine.tempDirs.chroot)
	if err != nil {
		return fmt.Errorf("Error reading unpack/chroot dir: %s", err.Error())
	}

	for _, srcFile := range files {
		srcFile := filepath.Join(stateMachine.tempDirs.chroot, srcFile.Name())
		if err := osutilCopySpecialFile(srcFile, classicStateMachine.tempDirs.rootfs); err != nil {
			return fmt.Errorf("Error copying rootfs: %s", err.Error())
		}
	}

	if classicStateMachine.ImageDef.Customization == nil {
		return nil
	}

	return classicStateMachine.fixFstab()
}

// customizeSourcesList customize the /etc/apt/sources.list file for the
// resulting image. This state must be executed once packages installation
// is done, and before other manual customization to let users modify it.
func (stateMachine *StateMachine) customizeSourcesList() error {
	classicStateMachine := stateMachine.parent.(*ClassicStateMachine)

	if *classicStateMachine.ImageDef.Rootfs.SourcesListDeb822 {
		err := stateMachine.setDeb822SourcesList(classicStateMachine.ImageDef.Deb822TargetSourcesList())
		if err != nil {
			return err
		}
		return stateMachine.setLegacySourcesList(imagedefinition.LegacySourcesListComment)
	}

	return stateMachine.setLegacySourcesList(classicStateMachine.ImageDef.LegacyTargetSourcesList())
}

// setLegacySourcesList replaces /etc/apt/sources.list with the given list of entries
// This function will truncate the existing file.
func (stateMachine *StateMachine) setLegacySourcesList(aptSources string) error {
	sourcesList := filepath.Join(stateMachine.tempDirs.chroot, "etc", "apt", "sources.list")
	sourcesListFile, err := osOpenFile(sourcesList, os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("unable to open sources.list file: %w", err)
	}
	defer sourcesListFile.Close()
	_, err = sourcesListFile.WriteString(aptSources)
	if err != nil {
		return fmt.Errorf("unable to write apt sources: %w", err)
	}
	return nil
}

// setDeb822SourcesList replaces /etc/apt/sources.list.d/ubuntu.sources with the given content
// This function will truncate the existing file if any
func (stateMachine *StateMachine) setDeb822SourcesList(sourcesListContent string) error {
	sourcesListDir := filepath.Join(stateMachine.tempDirs.chroot, "etc", "apt", "sources.list.d")
	err := osMkdirAll(sourcesListDir, 0755)
	if err != nil && !os.IsExist(err) {
		return fmt.Errorf("Error /etc/apt/sources.list.d directory: %s", err.Error())
	}

	sourcesList := filepath.Join(sourcesListDir, "ubuntu.sources")
	f, err := osOpenFile(sourcesList, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("unable to open ubuntu.sources file: %w", err)
	}
	defer f.Close()

	_, err = f.WriteString(sourcesListContent)
	if err != nil {
		return fmt.Errorf("unable to write apt sources: %w", err)
	}

	return nil
}

// fixFstab makes sure the fstab contains a valid entry for the root mount point
func (stateMachine *StateMachine) fixFstab() error {
	classicStateMachine := stateMachine.parent.(*ClassicStateMachine)

	if len(classicStateMachine.ImageDef.Customization.Fstab) != 0 {
		return nil
	}

	fstabPath := filepath.Join(classicStateMachine.tempDirs.rootfs, "etc", "fstab")
	fstabBytes, err := osReadFile(fstabPath)
	if err != nil {
		return fmt.Errorf("Error reading fstab: %s", err.Error())
	}

	rootMountFound := false
	newLines := make([]string, 0)
	rootFSLabel := "writable"
	rootFSOptions := "discard,errors=remount-ro"
	fsckOrder := "1"

	lines := strings.Split(string(fstabBytes), "\n")
	for _, l := range lines {
		if l == "# UNCONFIGURED FSTAB" {
			// omit this line if still present
			continue
		}

		if strings.HasPrefix(l, "#") {
			newLines = append(newLines, l)
			continue
		}

		entry := strings.Fields(l)
		if len(entry) < 6 {
			// ignore invalid fstab entry
			continue
		}

		if entry[1] == "/" && !rootMountFound {
			entry[0] = "LABEL=" + rootFSLabel
			entry[3] = rootFSOptions
			entry[5] = fsckOrder

			rootMountFound = true
		}
		newLines = append(newLines, strings.Join(entry, "\t"))
	}

	if !rootMountFound {
		newLines = append(newLines, fmt.Sprintf("LABEL=%s	/	ext4	%s	0	%s", rootFSLabel, rootFSOptions, fsckOrder))
	}

	err = osWriteFile(fstabPath, []byte(strings.Join(newLines, "\n")+"\n"), 0644)
	if err != nil {
		return fmt.Errorf("Error writing to fstab: %s", err.Error())
	}
	return nil
}

// Set a default locale if one is not configured beforehand by other customizations
func (stateMachine *StateMachine) setDefaultLocale() error {
	classicStateMachine := stateMachine.parent.(*ClassicStateMachine)

	defaultPath := filepath.Join(classicStateMachine.tempDirs.chroot, "etc", "default")
	localePath := filepath.Join(defaultPath, "locale")
	localeBytes, err := osReadFile(localePath)
	if err == nil && localePresentRegex.Find(localeBytes) != nil {
		return nil
	}

	err = osMkdirAll(defaultPath, 0755)
	if err != nil {
		return fmt.Errorf("Error creating default directory: %s", err.Error())
	}

	err = osWriteFile(localePath, []byte("# Default Ubuntu locale\nLANG=C.UTF-8\n"), 0644)
	if err != nil {
		return fmt.Errorf("Error writing to locale file: %s", err.Error())
	}
	return nil
}

// Generate the manifest
func (stateMachine *StateMachine) generatePackageManifest() error {
	classicStateMachine := stateMachine.parent.(*ClassicStateMachine)

	// This is basically just a wrapper around dpkg-query
	outputPath := filepath.Join(stateMachine.commonFlags.OutputDir,
		classicStateMachine.ImageDef.Artifacts.Manifest.ManifestName)
	cmd := execCommand("chroot", stateMachine.tempDirs.rootfs, "dpkg-query", "-W", "--showformat=${Package} ${Version}\n")
	cmdOutput := helper.SetCommandOutput(cmd, classicStateMachine.commonFlags.Debug)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Error generating package manifest with command \"%s\". "+
			"Error is \"%s\". Full output below:\n%s",
			cmd.String(), err.Error(), cmdOutput.String())
	}

	// write the output to a file on successful executions
	manifest, err := osCreate(outputPath)
	if err != nil {
		return fmt.Errorf("Error creating manifest file: %s", err.Error())
	}
	defer manifest.Close()
	_, err = manifest.Write(cmdOutput.Bytes())
	if err != nil {
		return fmt.Errorf("error writing the manifest file: %w", err)
	}
	return nil
}

// Generate the manifest
func (stateMachine *StateMachine) generateFilelist() error {
	classicStateMachine := stateMachine.parent.(*ClassicStateMachine)

	// This is basically just a wrapper around find (similar to what we do in livecd-rootfs)
	outputPath := filepath.Join(stateMachine.commonFlags.OutputDir,
		classicStateMachine.ImageDef.Artifacts.Filelist.FilelistName)
	cmd := execCommand("chroot", stateMachine.tempDirs.rootfs, "find", "-xdev")
	cmdOutput := helper.SetCommandOutput(cmd, classicStateMachine.commonFlags.Debug)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Error generating file list with command \"%s\". "+
			"Error is \"%s\". Full output below:\n%s",
			cmd.String(), err.Error(), cmdOutput.String())
	}

	// write the output to a file on successful executions
	filelist, err := osCreate(outputPath)
	if err != nil {
		return fmt.Errorf("Error creating filelist file: %s", err.Error())
	}
	defer filelist.Close()
	_, err = filelist.Write(cmdOutput.Bytes())
	if err != nil {
		return fmt.Errorf("error writing the filelist file: %w", err)
	}
	return nil
}

// Generate the rootfs tarball
func (stateMachine *StateMachine) generateRootfsTarball() error {
	classicStateMachine := stateMachine.parent.(*ClassicStateMachine)

	// first create a vanilla uncompressed tar archive
	rootfsSrc := filepath.Join(stateMachine.stateMachineFlags.WorkDir, "root")
	rootfsDst := filepath.Join(stateMachine.commonFlags.OutputDir,
		classicStateMachine.ImageDef.Artifacts.RootfsTar.RootfsTarName)
	return helper.CreateTarArchive(rootfsSrc, rootfsDst,
		classicStateMachine.ImageDef.Artifacts.RootfsTar.Compression,
		stateMachine.commonFlags.Verbose, stateMachine.commonFlags.Debug)
}

// makeQcow2Img converts raw .img artifacts into qcow2 artifacts
func (stateMachine *StateMachine) makeQcow2Img() error {
	classicStateMachine := stateMachine.parent.(*ClassicStateMachine)

	for _, qcow2 := range *classicStateMachine.ImageDef.Artifacts.Qcow2 {
		backingFile := filepath.Join(stateMachine.commonFlags.OutputDir, stateMachine.VolumeNames[qcow2.Qcow2Volume])
		resultingFile := filepath.Join(stateMachine.commonFlags.OutputDir, qcow2.Qcow2Name)
		qemuImgCommand := execCommand("qemu-img",
			"convert",
			"-c",
			"-O",
			"qcow2",
			"-o",
			"compat=0.10",
			backingFile,
			resultingFile,
		)
		qemuOutput := helper.SetCommandOutput(qemuImgCommand, classicStateMachine.commonFlags.Debug)
		if err := qemuImgCommand.Run(); err != nil {
			return fmt.Errorf("Error creating qcow2 artifact with command \"%s\". "+
				"Error is \"%s\". Full output below:\n%s",
				qemuImgCommand.String(), err.Error(), qemuOutput.String())
		}
	}
	return nil
}

// updateBootloader determines the bootloader for each volume
// and runs the correct helper function to update the bootloader
func (stateMachine *StateMachine) updateBootloader() error {
	if stateMachine.rootfsPartNum == -1 || stateMachine.rootfsVolName == "" {
		return fmt.Errorf("Error: could not determine partition number of the root filesystem")
	}
	volume := stateMachine.GadgetInfo.Volumes[stateMachine.rootfsVolName]
	switch volume.Bootloader {
	case "grub":
		err := stateMachine.updateGrub(stateMachine.rootfsVolName, stateMachine.rootfsPartNum)
		if err != nil {
			return err
		}
	default:
		fmt.Printf("WARNING: updating bootloader %s not yet supported\n",
			volume.Bootloader,
		)
	}
	return nil
}

// cleanRootfs cleans the created chroot from secrets/values generated
// during the various preceding install steps
func (stateMachine *StateMachine) cleanRootfs() error {
	toClean := []string{
		filepath.Join(stateMachine.tempDirs.chroot, "var", "lib", "dbus", "machine-id"),
	}

	toTruncate := []string{
		filepath.Join(stateMachine.tempDirs.chroot, "etc", "machine-id"),
	}

	// openssh default keys
	sshPubKeys, err := filepath.Glob(filepath.Join(stateMachine.tempDirs.chroot, "etc", "ssh", "ssh_host_*_key.pub"))
	if err != nil {
		return fmt.Errorf("unable to list ssh pub keys: %s", err.Error())
	}

	toClean = append(toClean, sshPubKeys...)

	sshPrivKeys, err := filepath.Glob(filepath.Join(stateMachine.tempDirs.chroot, "etc", "ssh", "ssh_host_*_key"))
	if err != nil {
		return fmt.Errorf("unable to list ssh pub keys: %s", err.Error())
	}

	toClean = append(toClean, sshPrivKeys...)

	oldDebconf, err := filepath.Glob(filepath.Join(stateMachine.tempDirs.chroot, "var", "cache", "debconf", "*-old"))
	if err != nil {
		return fmt.Errorf("unable to list old debconf conf: %s", err.Error())
	}

	toClean = append(toClean, oldDebconf...)

	oldDpkg, err := filepath.Glob(filepath.Join(stateMachine.tempDirs.chroot, "var", "lib", "dpkg", "*-old"))
	if err != nil {
		return fmt.Errorf("unable to list old dpkg conf: %s", err.Error())
	}

	toClean = append(toClean, oldDpkg...)

	for _, f := range toClean {
		err = osRemove(f)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("Error removing %s: %s", f, err.Error())
		}
	}

	// udev persistent rules
	udevRules, err := filepath.Glob(filepath.Join(stateMachine.tempDirs.chroot, "etc", "udev", "rules.d", "*persistent-net.rules"))
	if err != nil {
		return fmt.Errorf("unable to list udev persistent rules: %s", err.Error())
	}

	toTruncate = append(toTruncate, udevRules...)

	for _, f := range toTruncate {
		err = osTruncate(f, 0)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("Error truncating %s: %s", f, err.Error())
		}
	}

	return nil
}
