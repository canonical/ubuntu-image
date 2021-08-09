package helper

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/canonical/ubuntu-image/internal/commands"
	"github.com/snapcore/snapd/gadget/quantity"
	"github.com/snapcore/snapd/osutil"
)

// CaptureStd returns an io.Reader to read what was printed, and teardown
func CaptureStd(toCap **os.File) (io.Reader, func(), error) {
	stdCap, stdCapW, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	oldStdCap := *toCap
	*toCap = stdCapW
	closed := false
	return stdCap, func() {
		// only teardown once
		if closed {
			return
		}
		*toCap = oldStdCap
		stdCapW.Close()
		closed = true
	}, nil
}

// InitCommonOpts initializes default common options for state machines.
// This is used for test scenarios to avoid nil pointer dereferences
func InitCommonOpts() (*commands.CommonOpts, *commands.StateMachineOpts) {
	return new(commands.CommonOpts), new(commands.StateMachineOpts)
}

// GetHostArch uses dpkg to return the host architecture of the current system
func GetHostArch() string {
	cmd := exec.Command("dpkg", "--print-architecture")
	outputBytes, _ := cmd.Output()
	return strings.TrimSpace(string(outputBytes))
}

// getQemuStaticForArch returns the name of the qemu binary for the specified arch
func getQemuStaticForArch(arch string) string {
	archs := map[string]string{
		"armhf":   "qemu-arm-static",
		"arm64":   "qemu-aarch64-static",
		"ppc64el": "qemu-ppc64le-static",
	}
	if static, exists := archs[arch]; exists {
		return static
	}
	return ""
}

// SetupLiveBuildCommands creates the live build commands used in classic images
func SetupLiveBuildCommands(rootfs, arch string, env []string, enableCrossBuild bool) (lbConfig, lbBuild exec.Cmd, err error) {

	lbConfig = *exec.Command("lb", "config")
	lbBuild = *exec.Command("lb", "build")

	lbConfig.Stdout = os.Stdout
	lbConfig.Stderr = os.Stderr
	lbConfig.Env = append(os.Environ(), env...)
	lbBuild.Stdout = os.Stdout
	lbBuild.Stderr = os.Stderr
	lbBuild.Env = append(os.Environ(), env...)

	autoSrc := os.Getenv("UBUNTU_IMAGE_LIVECD_ROOTFS_AUTO_PATH")
	if autoSrc == "" {
		dpkgArgs := "dpkg -L livecd-rootfs | grep \"auto$\""
		dpkgCommand := *exec.Command("bash", "-c", dpkgArgs)
		dpkgBytes, err := dpkgCommand.Output()
		if err != nil {
			return lbConfig, lbBuild, err
		}
		autoSrc = strings.TrimSpace(string(dpkgBytes))
	}
	autoDst := rootfs + "/auto"
	if err := osutil.CopySpecialFile(autoSrc, autoDst); err != nil {
		return lbConfig, lbBuild, fmt.Errorf("Error copying livecd-rootfs/auto: %s", err.Error())
	}

	if arch != GetHostArch() && enableCrossBuild {
		// For cases where we want to cross-build, we need to pass
		// additional options to live-build with the arch to use and path
		// to the qemu static
		var qemuPath string
		qemuPath = os.Getenv("UBUNTU_IMAGE_QEMU_USER_STATIC_PATH")
		if qemuPath == "" {
			static := getQemuStaticForArch(arch)
			qemuPath, err = exec.LookPath(static)
			if err != nil {
				return lbConfig, lbBuild, fmt.Errorf("Use " +
					"UBUNTU_IMAGE_QEMU_USER_STATIC_PATH in case " +
					"of non-standard archs or custom paths")
			}
		}
		lbConfig.Args = append(lbConfig.Args, []string{"--bootstrap-qemu-arch", arch, "--bootstrap-qemu-static", qemuPath, "--architectures", arch}...)
	}

	return lbConfig, lbBuild, nil
}

// RunScript sets up and runs the hookscript command
func RunScript(hookScript string) error {
	hookScriptCmd := exec.Command(hookScript)
	hookScriptCmd.Env = os.Environ()
	hookScriptCmd.Stdout = os.Stdout
	hookScriptCmd.Stderr = os.Stderr
	fmt.Println(hookScriptCmd)
	if err := hookScriptCmd.Run(); err != nil {
		return fmt.Errorf("Error running hook script %s: %s", hookScript, err.Error())
	}
	return nil
}

// GetHostSuite checks the release name of the host system to use as a default if --suite is not passed
func GetHostSuite() string {
	cmd := exec.Command("lsb_release", "-c", "-s")
	outputBytes, _ := cmd.Output()
	return strings.TrimSpace(string(outputBytes))
}

// SaveCWD gets the current working directory and returns a function to go back to it
func SaveCWD() func() {
	wd, _ := os.Getwd()
	return func() {
		os.Chdir(wd)
	}
}

// Du recurses through a directory similar to du and adds all the sizes of files together
func Du(path string) (quantity.Size, error) {
	var size quantity.Size = 0
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += quantity.Size(info.Size())
		}
		return err
	})
	return size, err
}

// MaxOffset returns the maximum of two quantity.Offset types
func MaxOffset(offset1, offset2 quantity.Offset) quantity.Offset {
	if offset1 > offset2 {
		return offset1
	}
	return offset2
}

// CopyBlob runs `dd` to copy a blob to an image file
func CopyBlob(ddArgs []string) error {
	ddCommand := *exec.Command("dd")
	ddCommand.Args = ddArgs
	fmt.Println(ddCommand)

	if err := ddCommand.Run(); err != nil {
		return err
	}
	return nil
}

// TODO: yeet this after getting the snapd code merged
type MkfsFunc func(imgFile, label, contentsRootDir string, deviceSize, sectorSize quantity.Size) error

var (
	mkfsHandlers = map[string]MkfsFunc{
		"vfat": mkfsVfat,
		"ext4": mkfsExt4,
	}
)

// Mkfs creates a filesystem of given type and provided label in the device or
// file. The device size and sector size provides hints for additional tuning of
// the created filesystem.
func Mkfs(typ, img, label string, deviceSize, sectorSize quantity.Size) error {
	return MkfsWithContent(typ, img, label, "", deviceSize, sectorSize)
}

// MkfsWithContent creates a filesystem of given type and provided label in the
// device or file. The filesystem is populated with contents of contentRootDir.
// The device size provides hints for additional tuning of the created
// filesystem.
func MkfsWithContent(typ, img, label, contentRootDir string, deviceSize, sectorSize quantity.Size) error {
	h, ok := mkfsHandlers[typ]
	if !ok {
		return fmt.Errorf("cannot create unsupported filesystem %q", typ)
	}
	return h(img, label, contentRootDir, deviceSize, sectorSize)
}

// mkfsExt4 creates an EXT4 filesystem in given image file, with an optional
// filesystem label, and populates it with the contents of provided root
// directory.
func mkfsExt4(img, label, contentsRootDir string, deviceSize, sectorSize quantity.Size) error {
	// Originally taken from ubuntu-image
	// Switched to use mkfs defaults for https://bugs.launchpad.net/snappy/+bug/1878374
	// For caveats/requirements in case we need support for older systems:
	// https://github.com/snapcore/snapd/pull/6997#discussion_r293967140
	mkfsArgs := []string{"mkfs.ext4"}

	const size32MiB = 32 * quantity.SizeMiB
	if deviceSize != 0 && deviceSize <= size32MiB {
		// With the default block size of 4096 bytes, the minimal journal size
		// is 4M, meaning we loose a lot of usable space. Try to follow the
		// e2fsprogs upstream and use a 1k block size for smaller
		// filesystems, note that this may cause issues like
		// https://bugs.launchpad.net/ubuntu/+source/lvm2/+bug/1817097
		// if one migrates the filesystem to a device with a different
		// block size

		// though note if the sector size was specified (i.e. non-zero) and
		// larger than 1K, then we need to use that, since you can't create
		// a filesystem with a block-size smaller than the sector-size
		// see e2fsprogs source code:
		// https://github.com/tytso/e2fsprogs/blob/0d47f5ab05177c1861f16bb3644a47018e6be1d0/misc/mke2fs.c#L2151-L2156
		defaultSectorSize := 1 * quantity.SizeKiB
		if sectorSize > 1024 {
			defaultSectorSize = sectorSize
		}
		mkfsArgs = append(mkfsArgs, "-b", defaultSectorSize.String())
	}
	if contentsRootDir != "" {
		// mkfs.ext4 can populate the filesystem with contents of given
		// root directory
		// TODO: support e2fsprogs 1.42 without -d in Ubuntu 16.04
		mkfsArgs = append(mkfsArgs, "-d", contentsRootDir)
	}
	if label != "" {
		mkfsArgs = append(mkfsArgs, "-L", label)
	}
	mkfsArgs = append(mkfsArgs, img)

	var cmd *exec.Cmd
	if os.Geteuid() != 0 {
		// run through fakeroot so that files are owned by root
		cmd = exec.Command("fakeroot", mkfsArgs...)
	} else {
		// no need to fake it if we're already root
		cmd = exec.Command(mkfsArgs[0], mkfsArgs[1:]...)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return osutil.OutputErr(out, err)
	}
	return nil
}

// mkfsVfat creates a VFAT filesystem in given image file, with an optional
// filesystem label, and populates it with the contents of provided root
// directory.
func mkfsVfat(img, label, contentsRootDir string, deviceSize, sectorSize quantity.Size) error {
	// 512B logical sector size by default, unless the specified sector size is
	// larger than 512, in which case use the sector size
	// mkfs.vfat will automatically increase the block size to the internal
	// sector size of the disk if the specified block size is too small, but
	// be paranoid and always set the block size to that of the sector size if
	// we know the sector size is larger than the default 512 (originally from
	// ubuntu-image). see dosfstools:
	// https://github.com/dosfstools/dosfstools/blob/e579a7df89bb3a6df08847d45c70c8ebfabca7d2/src/mkfs.fat.c#L1892-L1898
	defaultSectorSize := quantity.Size(512)
	if sectorSize > defaultSectorSize {
		defaultSectorSize = sectorSize
	}
	mkfsArgs := []string{
		// options taken from ubuntu-image, except the sector size
		"-S", defaultSectorSize.String(),
		// 1 sector per cluster
		"-s", "1",
		// 32b FAT size
		"-F", "32",
	}
	if label != "" {
		mkfsArgs = append(mkfsArgs, "-n", label)
	}
	mkfsArgs = append(mkfsArgs, img)

	cmd := exec.Command("mkfs.vfat", mkfsArgs...)
	fmt.Println(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return osutil.OutputErr(out, err)
	}

	// if there is no content to copy we are done now
	if contentsRootDir == "" {
		return nil
	}

	// mkfs.vfat does not know how to populate the filesystem with contents,
	// we need to do the work ourselves

	fis, err := ioutil.ReadDir(contentsRootDir)
	if err != nil {
		return fmt.Errorf("cannot list directory contents: %v", err)
	}
	if len(fis) == 0 {
		// nothing to copy to the image
		return nil
	}

	mcopyArgs := make([]string, 0, 4+len(fis))
	mcopyArgs = append(mcopyArgs,
		// recursive copy
		"-s",
		// image file
		"-i", img)
	for _, fi := range fis {
		mcopyArgs = append(mcopyArgs, filepath.Join(contentsRootDir, fi.Name()))
	}
	mcopyArgs = append(mcopyArgs,
		// place content at the / of the filesystem
		"::")

	cmd = exec.Command("mcopy", mcopyArgs...)
	cmd.Env = os.Environ()
	// skip mtools checks to avoid unnecessary warnings
	cmd.Env = append(cmd.Env, "MTOOLS_SKIP_CHECK=1")

	out, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("cannot populate vfat filesystem with contents: %v", osutil.OutputErr(out, err))
	}
	return nil
}
