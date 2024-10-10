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
	"github.com/canonical/ubuntu-image/internal/ppa"
)

var (
	seedVersionRegex   = regexp.MustCompile(`^[a-z0-9].*`)
	localePresentRegex = regexp.MustCompile(`(?m)^LANG=|LC_[A-Z_]+=`)
)

var buildGadgetTreeState = stateFunc{"build_gadget_tree", (*StateMachine).buildGadgetTree}

// Build the gadget tree
func (stateMachine *StateMachine) buildGadgetTree() error {
	classicStateMachine := stateMachine.parent.(*ClassicStateMachine)

	// make the gadget directory under scratch
	gadgetDir := filepath.Join(stateMachine.tempDirs.scratch, "gadget")

	err := classicStateMachine.prepareGadgetDir(gadgetDir)
	if err != nil {
		return err
	}

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

// prepareGadgetDir prepares the gadget directory prior to running the make command
func (classicStateMachine *ClassicStateMachine) prepareGadgetDir(gadgetDir string) error {
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
			gadgetTreePath = filepath.Join(classicStateMachine.ConfDefPath, gadgetTreePath)
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
	return nil
}

var prepareGadgetTreeState = stateFunc{"prepare_gadget_tree", (*StateMachine).prepareGadgetTree}

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
			gadgetTree, err = filepath.Abs(gadgetTree)
			if err != nil {
				return fmt.Errorf("Error finding the absolute path of the gadget tree: %s", err.Error())
			}
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

var createChrootState = stateFunc{"create_chroot", (*StateMachine).createChroot}

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

var addExtraPPAsState = stateFunc{"add_extra_ppas", (*StateMachine).addExtraPPAs}

// addExtraPPAs adds PPAs to the /etc/apt/sources.list.d directory
func (stateMachine *StateMachine) addExtraPPAs() (err error) {
	classicStateMachine := stateMachine.parent.(*ClassicStateMachine)

	for _, extraPPA := range classicStateMachine.ImageDef.Customization.ExtraPPAs {
		p := ppa.New(extraPPA, *classicStateMachine.ImageDef.Rootfs.SourcesListDeb822, classicStateMachine.ImageDef.Series)
		err := p.Add(classicStateMachine.tempDirs.chroot, classicStateMachine.commonFlags.Debug)
		if err != nil {
			return err
		}
	}

	return nil
}

var cleanExtraPPAsState = stateFunc{"clean_extra_ppas", (*StateMachine).cleanExtraPPAs}

// cleanExtraPPAs cleans previously added PPA to the source list
func (stateMachine *StateMachine) cleanExtraPPAs() (err error) {
	classicStateMachine := stateMachine.parent.(*ClassicStateMachine)

	for _, extraPPA := range classicStateMachine.ImageDef.Customization.ExtraPPAs {
		p := ppa.New(extraPPA, *classicStateMachine.ImageDef.Rootfs.SourcesListDeb822, classicStateMachine.ImageDef.Series)
		err := p.Remove(stateMachine.tempDirs.chroot)
		if err != nil {
			return err
		}
	}

	return nil
}

var installPackagesState = stateFunc{"install_packages", (*StateMachine).installPackages}

// Install packages in the chroot environment
func (stateMachine *StateMachine) installPackages() error {
	classicStateMachine := stateMachine.parent.(*ClassicStateMachine)

	err := helperBackupAndCopyResolvConf(classicStateMachine.tempDirs.chroot)
	if err != nil {
		return fmt.Errorf("Error setting up /etc/resolv.conf in the chroot: \"%s\"", err.Error())
	}

	stateMachine.gatherPackages(&classicStateMachine.ImageDef)

	// setupCmds should be filled as a FIFO list
	var setupCmds []*exec.Cmd

	// teardownCmds should be filled as a LIFO list
	var teardownCmds []*exec.Cmd

	mountPoints := []*mountPoint{}

	// Make sure we left the system as clean as possible if something has gone wrong
	defer func() {
		err = teardownMount(stateMachine.tempDirs.chroot, mountPoints, teardownCmds, err, stateMachine.commonFlags.Debug)
	}()

	// mount some necessary partitions in the chroot
	mountPoints = append(mountPoints,
		&mountPoint{
			src:      "devtmpfs-build",
			basePath: stateMachine.tempDirs.chroot,
			relpath:  "/dev",
			typ:      "devtmpfs",
		},
		&mountPoint{
			src:      "devpts-build",
			basePath: stateMachine.tempDirs.chroot,
			relpath:  "/dev/pts",
			typ:      "devpts",
			opts:     []string{"nodev", "nosuid"},
		},
		&mountPoint{
			src:      "proc-build",
			basePath: stateMachine.tempDirs.chroot,
			relpath:  "/proc",
			typ:      "proc",
		},
		&mountPoint{
			src:      "sysfs-build",
			basePath: stateMachine.tempDirs.chroot,
			relpath:  "/sys",
			typ:      "sysfs",
		},
		&mountPoint{
			basePath: stateMachine.tempDirs.chroot,
			relpath:  "/run",
			bind:     true,
		},
	)

	mountCmds, umountCmds, err := generateMountPointCmds(mountPoints, stateMachine.tempDirs.scratch)
	if err != nil {
		return err
	}
	setupCmds = append(setupCmds, mountCmds...)
	teardownCmds = append(umountCmds, teardownCmds...)

	teardownCmds = append([]*exec.Cmd{
		execCommand("udevadm", "settle"),
	}, teardownCmds...)

	policyRcDPath := filepath.Join(classicStateMachine.tempDirs.chroot, "usr", "sbin", "policy-rc.d")

	if osutil.FileExists(policyRcDPath) {
		divertCmd, undivertCmd := divertPolicyRcD(stateMachine.tempDirs.chroot)
		setupCmds = append(setupCmds, divertCmd)
		teardownCmds = append([]*exec.Cmd{undivertCmd}, teardownCmds...)
	}

	err = helper.RunCmds(setupCmds, classicStateMachine.commonFlags.Debug)
	if err != nil {
		return err
	}

	unsetDenyingPolicyRcD, err := setDenyingPolicyRcD(policyRcDPath)
	if err != nil {
		return err
	}

	defer func() {
		err = unsetDenyingPolicyRcD(err)
	}()

	restoreStartStopDaemon, err := backupReplaceStartStopDaemon(classicStateMachine.tempDirs.chroot)
	if err != nil {
		return err
	}

	defer func() {
		err = restoreStartStopDaemon(err)
	}()

	initctlPath := filepath.Join(classicStateMachine.tempDirs.chroot, "sbin", "initctl")

	if osutil.FileExists(initctlPath) {
		restoreInitctl, err := backupReplaceInitctl(classicStateMachine.tempDirs.chroot)
		if err != nil {
			return err
		}

		defer func() {
			err = restoreInitctl(err)
		}()
	}

	installPackagesCmds := generateAptCmds(stateMachine.tempDirs.chroot, classicStateMachine.Packages)

	err = helper.RunCmds(installPackagesCmds, classicStateMachine.commonFlags.Debug)
	if err != nil {
		return err
	}

	return nil
}

func (stateMachine *StateMachine) gatherPackages(imageDef *imagedefinition.ImageDefinition) {
	if imageDef.Customization != nil {
		for _, packageInfo := range imageDef.Customization.ExtraPackages {
			stateMachine.Packages = append(stateMachine.Packages,
				packageInfo.PackageName)
		}
	}

	// Make sure to install the extra kernel if it is specified
	if imageDef.Kernel != "" {
		stateMachine.Packages = append(stateMachine.Packages,
			imageDef.Kernel)
	}
}

// generateMountPointCmds generate lists of mount/umount commands for a list of mountpoints
func generateMountPointCmds(mountPoints []*mountPoint, scratchDir string) (allMountCmds []*exec.Cmd, allUmountCmds []*exec.Cmd, err error) {
	for _, mp := range mountPoints {
		var mountCmds, umountCmds []*exec.Cmd
		var err error
		if mp.bind {
			mp.src, err = osMkdirTemp(scratchDir, strings.Trim(mp.relpath, "/"))
			if err != nil {
				return nil, nil, fmt.Errorf("Error making temporary directory for mountpoint \"%s\": \"%s\"",
					mp.relpath,
					err.Error(),
				)
			}
		}

		mountCmds, umountCmds, err = mp.getMountCmd()
		if err != nil {
			return nil, nil, fmt.Errorf("Error preparing mountpoint \"%s\": \"%s\"",
				mp.relpath,
				err.Error(),
			)
		}

		allMountCmds = append(allMountCmds, mountCmds...)
		allUmountCmds = append(umountCmds, allUmountCmds...)
	}
	return allMountCmds, allUmountCmds, err
}

var verifyArtifactNamesState = stateFunc{"verify_artifact_names", (*StateMachine).verifyArtifactNames}

// Verify artifact names have volumes listed for multi-volume gadgets and set
// the volume names in the struct
func (stateMachine *StateMachine) verifyArtifactNames() error {
	classicStateMachine := stateMachine.parent.(*ClassicStateMachine)

	if classicStateMachine.ImageDef.Artifacts == nil {
		return nil
	}

	stateMachine.VolumeNames = make(map[string]string)

	if len(stateMachine.GadgetInfo.Volumes) > 1 {
		err := stateMachine.prepareImgArtifactsMultipleVolumes(classicStateMachine.ImageDef.Artifacts)
		if err != nil {
			return err
		}
		err = stateMachine.prepareQcow2ArtifactsMultipleVolumes(classicStateMachine.ImageDef.Artifacts)
		if err != nil {
			return err
		}
	} else {
		stateMachine.prepareImgArtifactOneVolume(classicStateMachine.ImageDef.Artifacts)
		stateMachine.prepareQcow2ArtifactOneVolume(classicStateMachine.ImageDef.Artifacts)
	}
	return nil
}

func (stateMachine *StateMachine) prepareImgArtifactsMultipleVolumes(artifacts *imagedefinition.Artifact) error {
	if artifacts.Img == nil {
		return nil
	}
	for _, img := range *artifacts.Img {
		if img.ImgVolume == "" {
			return fmt.Errorf("Volume names must be specified for each image when using a gadget with more than one volume")
		}
		stateMachine.VolumeNames[img.ImgVolume] = img.ImgName
	}
	return nil
}

// qcow2 img logic is complicated. If .img artifacts are already specified
// in the image definition for corresponding volumes, we will re-use those and
// convert them to a qcow2 image. Otherwise, we will create a raw .img file to
// use as an input file for the conversion.
// The names of these images are placed in the VolumeNames map, which is used
// as an input file for an eventual `qemu-convert` operation.
func (stateMachine *StateMachine) prepareQcow2ArtifactsMultipleVolumes(artifacts *imagedefinition.Artifact) error {
	if artifacts.Qcow2 == nil {
		return nil
	}
	for _, qcow2 := range *artifacts.Qcow2 {
		if qcow2.Qcow2Volume == "" {
			return fmt.Errorf("Volume names must be specified for each image when using a gadget with more than one volume")
		}
		// We can save a whole lot of disk I/O here if the volume is
		// already specified as a .img file
		if artifacts.Img != nil {
			found := false
			for _, img := range *artifacts.Img {
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
	return nil
}

func (stateMachine *StateMachine) prepareImgArtifactOneVolume(artifacts *imagedefinition.Artifact) {
	if artifacts.Img == nil {
		return
	}
	img := (*artifacts.Img)[0]
	if img.ImgVolume == "" {
		// there is only one volume, so get it from the map
		volName := reflect.ValueOf(stateMachine.GadgetInfo.Volumes).MapKeys()[0].String()
		stateMachine.VolumeNames[volName] = img.ImgName
	} else {
		stateMachine.VolumeNames[img.ImgVolume] = img.ImgName
	}
}

// qcow2 img logic is complicated. If .img artifacts are already specified
// in the image definition for corresponding volumes, we will re-use those and
// convert them to a qcow2 image. Otherwise, we will create a raw .img file to
// use as an input file for the conversion.
// The names of these images are placed in the VolumeNames map, which is used
// as an input file for an eventual `qemu-convert` operation.
func (stateMachine *StateMachine) prepareQcow2ArtifactOneVolume(artifacts *imagedefinition.Artifact) {
	if artifacts.Qcow2 == nil {
		return
	}
	qcow2 := (*artifacts.Qcow2)[0]
	if qcow2.Qcow2Volume == "" {
		volName := reflect.ValueOf(stateMachine.GadgetInfo.Volumes).MapKeys()[0].String()
		if artifacts.Img != nil {
			qcow2.Qcow2Volume = volName
			(*artifacts.Qcow2)[0] = qcow2
			return // We will re-use the .img file in this case
		}
		// there is only one volume, so get it from the map
		stateMachine.VolumeNames[volName] = fmt.Sprintf("%s.img", qcow2.Qcow2Name)
		qcow2.Qcow2Volume = volName
		(*artifacts.Qcow2)[0] = qcow2
	} else {
		if artifacts.Img != nil {
			return // We will re-use the .img file in this case
		}
		stateMachine.VolumeNames[qcow2.Qcow2Volume] = fmt.Sprintf("%s.img", qcow2.Qcow2Name)
	}
}

var buildRootfsFromTasksState = stateFunc{"build_rootfs_from_tasks", (*StateMachine).buildRootfsFromTasks}

// Build a rootfs from a list of archive tasks
func (stateMachine *StateMachine) buildRootfsFromTasks() error {
	// currently a no-op pending implementation of the classic image redesign
	return nil
}

var extractRootfsTarState = stateFunc{"extract_rootfs_tar", (*StateMachine).extractRootfsTar}

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
	return helper.ExtractTarArchive(tarPath, stateMachine.tempDirs.chroot, stateMachine.commonFlags.Debug)
}

var germinateState = stateFunc{"germinate", (*StateMachine).germinate}

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

	pkgsFromSeed, err := packagesFromSeed(".seed", classicStateMachine.ImageDef.Rootfs.Seed.Names, germinateDir)
	if err != nil {
		return err
	}
	classicStateMachine.Packages = append(classicStateMachine.Packages, pkgsFromSeed...)

	snapsFromSeed, err := packagesFromSeed(".snaps", classicStateMachine.ImageDef.Rootfs.Seed.Names, germinateDir)
	if err != nil {
		return err
	}

	classicStateMachine.Snaps = addUniqueSnaps(classicStateMachine.Snaps, snapsFromSeed)

	return nil
}

// packagesFromSeed returns a list of packages/snaps from a germinated seed
func packagesFromSeed(ext string, seedNames []string, germinateDir string) ([]string, error) {
	var pkgs []string
	for _, fileName := range seedNames {
		seedFilePath := filepath.Join(germinateDir, fileName+ext)
		seedFile, err := osOpen(seedFilePath)
		if err != nil {
			return pkgs, fmt.Errorf("Error opening seed file %s: \"%s\"", seedFilePath, err.Error())
		}
		defer seedFile.Close()

		seedScanner := bufio.NewScanner(seedFile)
		for seedScanner.Scan() {
			seedLine := seedScanner.Bytes()
			if seedVersionRegex.Match(seedLine) {
				packageName := strings.Split(string(seedLine), " ")[0]
				pkgs = append(pkgs, packageName)
			}
		}
	}
	return pkgs, nil
}

// addUniqueSnaps returns a list of unique snaps
func addUniqueSnaps(currentSnaps []string, newSnaps []string) []string {
	m := make(map[string]bool)
	snaps := []string{}
	toDuplicate := append(currentSnaps, newSnaps...)

	for _, s := range toDuplicate {
		if m[s] {
			continue
		}
		m[s] = true
		snaps = append(snaps, s)
	}
	return snaps
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

var customizeCloudInitState = stateFunc{"customize_cloud_init", (*StateMachine).customizeCloudInit}

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

var customizeFstabState = stateFunc{"customize_fstab", (*StateMachine).customizeFstab}

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

var manualCustomizationState = stateFunc{"perform_manual_customization", (*StateMachine).manualCustomization}

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

var prepareClassicImageState = stateFunc{"prepare_image", (*StateMachine).prepareClassicImage}

// prepareClassicImage calls image.Prepare to stage snaps in classic images
func (stateMachine *StateMachine) prepareClassicImage() error {
	classicStateMachine := stateMachine.parent.(*ClassicStateMachine)
	imageOpts := &image.Options{}
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

	err = resetPreseeding(imageOpts, classicStateMachine.tempDirs.chroot, stateMachine.commonFlags.Debug, stateMachine.commonFlags.Verbose)
	if err != nil {
		return err
	}

	err = ensureSnapBasesInstalled(imageOpts)
	if err != nil {
		return err
	}

	err = addExtraSnaps(imageOpts, &classicStateMachine.ImageDef)
	if err != nil {
		return err
	}

	setModelFile(imageOpts, classicStateMachine.ImageDef.ModelAssertion, stateMachine.ConfDefPath)

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

	if err := imagePrepare(imageOpts); err != nil {
		return fmt.Errorf("Error preparing image: %s", err.Error())
	}

	return nil
}

// resetPreseeding checks if the rootfs is already preseeded and reset if necessary.
// This can happen when building from a rootfs tarball
func resetPreseeding(imageOpts *image.Options, chroot string, debug, verbose bool) error {
	if !osutil.FileExists(filepath.Join(chroot, "var", "lib", "snapd", "state.json")) {
		return nil
	}
	// first get a list of all preseeded snaps
	// seededSnaps maps the snap name and channel that was seeded
	preseededSnaps, err := getPreseededSnaps(chroot)
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
	if !debug && !verbose {
		oldPreseedStdout := preseed.Stdout
		preseed.Stdout = io.Discard
		defer func() {
			preseed.Stdout = oldPreseedStdout
		}()
	}
	// We need to use the snap-preseed binary for the reset as well, as using
	// preseed.ClassicReset() might leave us in a chroot jail
	cmd := execCommand("/usr/lib/snapd/snap-preseed", "--reset", chroot)
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("Error resetting preseeding in the chroot. Error is \"%s\"", err.Error())
	}

	return nil
}

// ensureSnapBasesInstalled iterates through the list of snaps and ensure that all
// of their bases are also set to be installed. Note we only do this for snaps that
// are seeded. Users are expected to specify all base and content provider snaps
// in the image definition.
func ensureSnapBasesInstalled(imageOpts *image.Options) error {
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
	return nil
}

// addExtraSnaps adds any extra snaps from the image definition to the list
// This should be done last to ensure the correct channels are being used
func addExtraSnaps(imageOpts *image.Options, imageDefinition *imagedefinition.ImageDefinition) error {
	if imageDefinition.Customization == nil || len(imageDefinition.Customization.ExtraSnaps) == 0 {
		return nil
	}

	imageOpts.SeedManifest = seedwriter.NewManifest()
	for _, extraSnap := range imageDefinition.Customization.ExtraSnaps {
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
			err := imageOpts.SeedManifest.SetAllowedSnapRevision(extraSnap.SnapName, snap.R(extraSnap.SnapRevision))
			if err != nil {
				return fmt.Errorf("error dealing with the extra snap %s: %w", extraSnap.SnapName, err)
			}
		}
	}

	return nil
}

// setModelFile sets the ModelFile based on the given ModelAssertion
func setModelFile(imageOpts *image.Options, modelAssertion string, confDefPath string) {
	modelAssertionPath := strings.TrimPrefix(modelAssertion, "file://")
	// if no explicit model assertion was given, keep empty ModelFile to let snapd fallback to default
	// model assertion
	if len(modelAssertionPath) != 0 {
		if !filepath.IsAbs(modelAssertionPath) {
			imageOpts.ModelFile = filepath.Join(confDefPath, modelAssertionPath)
		} else {
			imageOpts.ModelFile = modelAssertionPath
		}
	}
}

var preseedClassicImageState = stateFunc{"preseed_image", (*StateMachine).preseedClassicImage}

// preseedClassicImage preseeds the snaps that have already been staged in the chroot
func (stateMachine *StateMachine) preseedClassicImage() (err error) {
	classicStateMachine := stateMachine.parent.(*ClassicStateMachine)

	// preseedCmds should be filled as a FIFO list
	var preseedCmds []*exec.Cmd
	// teardownCmds should be filled as a LIFO list to unmount first what was mounted last
	var teardownCmds []*exec.Cmd

	// set up the mount commands
	mountPoints := []*mountPoint{
		{
			src:      "devtmpfs-build",
			basePath: stateMachine.tempDirs.chroot,
			relpath:  "/dev",
			typ:      "devtmpfs",
		},
		{
			src:      "devpts-build",
			basePath: stateMachine.tempDirs.chroot,
			relpath:  "/dev/pts",
			typ:      "devpts",
			opts:     []string{"nodev", "nosuid"},
		},
		{
			src:      "proc-build",
			basePath: stateMachine.tempDirs.chroot,
			relpath:  "/proc",
			typ:      "proc",
		},
		{
			src:      "none",
			basePath: stateMachine.tempDirs.chroot,
			relpath:  "/sys/kernel/security",
			typ:      "securityfs",
		},
		{
			src:      "none",
			basePath: stateMachine.tempDirs.chroot,
			relpath:  "/sys/fs/cgroup",
			typ:      "cgroup2",
		},
	}

	// Make sure we left the system as clean as possible if something has gone wrong
	defer func() {
		err = teardownMount(stateMachine.tempDirs.chroot, mountPoints, teardownCmds, err, stateMachine.commonFlags.Debug)
	}()

	for _, mp := range mountPoints {
		mountCmds, umountCmds, err := mp.getMountCmd()
		if err != nil {
			return fmt.Errorf("Error preparing mountpoint \"%s\": \"%s\"",
				mp.relpath,
				err.Error(),
			)
		}
		preseedCmds = append(preseedCmds, mountCmds...)
		teardownCmds = append(umountCmds, teardownCmds...)
	}

	teardownCmds = append([]*exec.Cmd{
		execCommand("udevadm", "settle"),
	}, teardownCmds...)

	preseedCmds = append(preseedCmds,
		//nolint:gosec,G204
		exec.Command("/usr/lib/snapd/snap-preseed", stateMachine.tempDirs.chroot),
	)

	err = helper.RunCmds(preseedCmds, classicStateMachine.commonFlags.Debug)
	if err != nil {
		return err
	}

	return nil
}

var populateClassicRootfsContentsState = stateFunc{"populate_rootfs_contents", (*StateMachine).populateClassicRootfsContents}

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
		return fmt.Errorf("Error reading chroot dir: %s", err.Error())
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

var customizeSourcesListState = stateFunc{"customize_sources_list", (*StateMachine).customizeSourcesList}

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

	lines := strings.Split(string(fstabBytes), "\n")
	newLines := generateFstabLines(lines)

	err = osWriteFile(fstabPath, []byte(strings.Join(newLines, "\n")+"\n"), 0644)
	if err != nil {
		return fmt.Errorf("Error writing to fstab: %s", err.Error())
	}
	return nil
}

// generateFstabLines generates new fstab lines from current ones
func generateFstabLines(lines []string) []string {
	rootMountFound := false
	newLines := make([]string, 0)
	rootFSLabel := "writable"
	rootFSOptions := "discard,errors=remount-ro"
	fsckOrder := "1"

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

	return newLines
}

var setDefaultLocaleState = stateFunc{"set_default_locale", (*StateMachine).setDefaultLocale}

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

var generatePackageManifestState = stateFunc{"generate_package_manifest", (*StateMachine).generatePackageManifest}

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

var generateFilelistState = stateFunc{"generate_filelist", (*StateMachine).generateFilelist}

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

var generateRootfsTarballState = stateFunc{"generate_rootfs_tarball", (*StateMachine).generateRootfsTarball}

// Generate the rootfs tarball
func (stateMachine *StateMachine) generateRootfsTarball() error {
	classicStateMachine := stateMachine.parent.(*ClassicStateMachine)

	tarDst := filepath.Join(
		stateMachine.commonFlags.OutputDir,
		classicStateMachine.ImageDef.Artifacts.RootfsTar.RootfsTarName,
	)
	return helper.CreateTarArchive(
		stateMachine.tempDirs.rootfs,
		tarDst,
		classicStateMachine.ImageDef.Artifacts.RootfsTar.Compression,
		stateMachine.commonFlags.Debug,
	)
}

var makeQcow2ImgState = stateFunc{"make_qcow2_image", (*StateMachine).makeQcow2Img}

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
			backingFile,
			resultingFile,
		)
		err := helper.RunCmd(qemuImgCommand, classicStateMachine.commonFlags.Debug)
		if err != nil {
			return err
		}
	}
	return nil
}

var updateBootloaderState = stateFunc{"update_bootloader", (*StateMachine).updateBootloader}

// updateBootloader determines the bootloader for each volume
// and runs the correct helper function to update the bootloader
func (stateMachine *StateMachine) updateBootloader() error {
	if stateMachine.RootfsPartNum == -1 || stateMachine.RootfsVolName == "" {
		return fmt.Errorf("Error: could not determine partition number of the root filesystem")
	}
	volume := stateMachine.GadgetInfo.Volumes[stateMachine.RootfsVolName]
	switch volume.Bootloader {
	case "grub":
		err := stateMachine.updateGrub(stateMachine.RootfsVolName, stateMachine.RootfsPartNum)
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

var cleanRootfsState = stateFunc{"clean_rootfs", (*StateMachine).cleanRootfs}

// cleanRootfs cleans the created chroot from secrets/values generated
// during the various preceding install steps
func (stateMachine *StateMachine) cleanRootfs() error {
	toDelete := []string{
		filepath.Join(stateMachine.tempDirs.chroot, "var", "lib", "dbus", "machine-id"),
	}

	toTruncate := []string{
		filepath.Join(stateMachine.tempDirs.chroot, "etc", "machine-id"),
	}

	toCleanFromPattern, err := listWithPatterns(stateMachine.tempDirs.chroot,
		[]string{
			filepath.Join("etc", "ssh", "ssh_host_*_key.pub"),
			filepath.Join("etc", "ssh", "ssh_host_*_key"),
			filepath.Join("var", "cache", "debconf", "*-old"),
			filepath.Join("var", "lib", "dpkg", "*-old"),
			filepath.Join("dev", "*"),
			filepath.Join("sys", "*"),
			filepath.Join("run", "*"),
		})
	if err != nil {
		return err
	}

	toDelete = append(toDelete, toCleanFromPattern...)

	err = doDeleteFiles(toDelete)
	if err != nil {
		return err
	}

	toTruncateFromPattern, err := listWithPatterns(stateMachine.tempDirs.chroot,
		[]string{
			// udev persistent rules
			filepath.Join("etc", "udev", "rules.d", "*persistent-net.rules"),
		})
	if err != nil {
		return err
	}

	toTruncate = append(toTruncate, toTruncateFromPattern...)

	return doTruncateFiles(toTruncate)
}

func listWithPatterns(chroot string, patterns []string) ([]string, error) {
	files := make([]string, 0)
	for _, pattern := range patterns {
		matches, err := filepath.Glob(filepath.Join(chroot, pattern))
		if err != nil {
			return nil, fmt.Errorf("unable to list files for pattern %s: %s", pattern, err.Error())
		}

		files = append(files, matches...)
	}
	return files, nil
}

// doDeleteFiles deletes the given list of files
func doDeleteFiles(toDelete []string) error {
	for _, f := range toDelete {
		err := osRemoveAll(f)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("Error removing %s: %s", f, err.Error())
		}
	}
	return nil
}

// doTruncateFiles truncates content in the given list of files
func doTruncateFiles(toTruncate []string) error {
	for _, f := range toTruncate {
		err := osTruncate(f, 0)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("Error truncating %s: %s", f, err.Error())
		}
	}
	return nil
}
