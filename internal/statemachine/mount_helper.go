package statemachine

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type mountPoint struct {
	src      string
	path     string
	basePath string // basePath + relpath = path
	relpath  string
	typ      string
	opts     []string
	bind     bool
}

// getMountCmd returns mount/umount commands to mount the given mountpoint
// If the mountpoint does not exist, it will be created.
func (m *mountPoint) getMountCmd() (mountCmds, umountCmds []*exec.Cmd, err error) {
	if m.bind && len(m.typ) > 0 {
		return nil, nil, fmt.Errorf("invalid mount arguments. Cannot use --bind and -t at the same time.")
	}

	targetPath := filepath.Join(m.basePath, m.relpath)
	mountCmd := execCommand("mount")

	if len(m.typ) > 0 {
		mountCmd.Args = append(mountCmd.Args, "-t", m.typ)
	}

	if m.bind {
		mountCmd.Args = append(mountCmd.Args, "--bind")
	}

	mountCmd.Args = append(mountCmd.Args, m.src)
	if len(m.opts) > 0 {
		mountCmd.Args = append(mountCmd.Args, "-o", strings.Join(m.opts, ","))
	}
	mountCmd.Args = append(mountCmd.Args, targetPath)

	if _, err := os.Stat(targetPath); err != nil {
		err := osMkdirAll(targetPath, 0755)
		if err != nil && !os.IsExist(err) {
			return nil, nil, fmt.Errorf("Error creating mountpoint \"%s\": \"%s\"", targetPath, err.Error())
		}
	}

	umountCmds = getUnmountCmd(targetPath)

	return []*exec.Cmd{mountCmd}, umountCmds, nil
}

// getUnmountCmd generates unmount commands from a path
func getUnmountCmd(targetPath string) []*exec.Cmd {
	return []*exec.Cmd{
		execCommand("mount", "--make-rprivate", targetPath),
		execCommand("umount", "--recursive", targetPath),
	}
}

// teardownMount executed teardown commands after making sure every mountpoints matching the given path
// are listed and will be properly unmounted
func teardownMount(path string, mountPoints []*mountPoint, teardownCmds []*exec.Cmd, err error, debug bool) error {
	addedUmountCmds, errAddedUmount := umountAddedMountPointsCmds(path, mountPoints)
	if errAddedUmount != nil {
		err = fmt.Errorf("%s\n%s", err, errAddedUmount)
	}
	teardownCmds = append(addedUmountCmds, teardownCmds...)

	return execTeardownCmds(teardownCmds, debug, err)
}

// umountAddedMountPointsCmds generates umount commands for newly added mountpoints
func umountAddedMountPointsCmds(path string, mountPoints []*mountPoint) (umountCmds []*exec.Cmd, err error) {
	currentMountPoints, err := listMounts(path)
	if err != nil {
		return nil, err
	}
	newMountPoints := diffMountPoints(mountPoints, currentMountPoints)
	if len(newMountPoints) > 0 {
		for _, m := range newMountPoints {
			umountCmds = append(umountCmds, getUnmountCmd(m.path)...)
		}
	}

	return umountCmds, nil
}

// diffMountPoints compares 2 lists of mountpoint and returns the added ones
func diffMountPoints(olds []*mountPoint, currents []*mountPoint) (added []*mountPoint) {
	for _, m := range currents {
		found := false
		for _, o := range olds {
			if equalMountPoints(*m, *o) {
				found = true
			}
		}
		if !found {
			added = append(added, m)
		}
	}

	return added
}

// equalMountPoints compare 2 mountpoints. Since mountPoints go object can be either
// created by hand or parsed from /proc/self/mount, we need to compare strictly on the fields
// useful to identify a unique mountpoint from the point of view of the OS.
func equalMountPoints(a, b mountPoint) bool {
	if len(a.path) == 0 {
		a.path = filepath.Join(a.basePath, a.relpath)
	}
	if len(b.path) == 0 {
		b.path = filepath.Join(b.basePath, b.relpath)
	}

	return a.src == b.src && a.path == b.path && a.typ == b.typ
}

// listMounts returns mountpoints matching the given path from /proc/self/mounts
func listMounts(path string) ([]*mountPoint, error) {
	procMounts := "/proc/self/mounts"
	f, err := osReadFile(procMounts)
	if err != nil {
		return nil, err
	}

	return parseMounts(string(f), path)
}

// parseMounts list existing mounts and submounts in the current path
// The returned splice is already inverted so unmount can be called on it
// without further modification.
func parseMounts(procMount string, path string) ([]*mountPoint, error) {
	mountPoints := []*mountPoint{}
	mountLines := strings.Split(procMount, "\n")

	for _, line := range mountLines {
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		mountPath := fields[1]

		if len(path) != 0 && !strings.HasPrefix(mountPath, path) {
			continue
		}

		m := &mountPoint{
			src:  fields[0],
			path: mountPath,
			typ:  fields[2],
			opts: strings.Split(fields[3], ","),
		}
		mountPoints = append([]*mountPoint{m}, mountPoints...)
	}

	return mountPoints, nil
}
