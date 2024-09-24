package arch

import (
	"runtime"
	"testing"
)

// TestGetHostArch unit tests the getHostArch function
func TestGetHostArch(t *testing.T) {
	t.Parallel()

	var expected string
	switch runtime.GOARCH {
	case "amd64":
		expected = "amd64"
	case "arm":
		expected = "armhf"
	case "arm64":
		expected = "arm64"
	case "ppc64le":
		expected = "ppc64el"
	case "s390x":
		expected = "s390x"
	case "riscv64":
		expected = "riscv64"
	default:
		t.Skipf("Test not supported on architecture %s", runtime.GOARCH)
	}

	hostArch := GetHostArch()
	if hostArch != expected {
		t.Errorf("Wrong value of getHostArch. Expected %s, got %s", expected, hostArch)
	}
}
