package arch

import (
	"os/exec"
	"strings"
)

const (
	AMD64   = "amd64"
	ARM64   = "arm64"
	ARMHF   = "armhf"
	I386    = "i386"
	PPC64EL = "ppc64el"
	S390X   = "s390x"
	RISCV64 = "riscv64"
)

// GetHostArch uses dpkg to return the host architecture of the current system
func GetHostArch() string {
	cmd := exec.Command("dpkg", "--print-architecture")
	outputBytes, _ := cmd.Output() // nolint: errcheck
	return strings.TrimSpace(string(outputBytes))
}
