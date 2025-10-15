package statemachine

import (
	"fmt"
	"os/exec"
	"strings"
)

// releaseFromCodename returns the release version associated to a codename
func releaseFromCodename(codename string) (string, error) {
	cmd := exec.Command("ubuntu-distro-info", "-r", "--series", codename)
	outputBytes, err := cmd.Output() // nolint: errcheck
	if err != nil {
		return "", fmt.Errorf("unable to get the release from the codename %s", codename)
	}
	// Remove the "LTS" suffix if needed
	return strings.TrimSpace(strings.Split(string(outputBytes), " ")[0]), nil
}

// isSeriesEqualOrOlder returns true if a is equal to or older than b
func isSeriesEqualOrOlder(a, b string) (bool, error) {
	if a == b {
		return true, nil
	}
	aRelease, err := releaseFromCodename(a)
	if err != nil {
		return false, err
	}
	bRelease, err := releaseFromCodename(b)
	if err != nil {
		return false, err
	}
	cmd := exec.Command("dpkg", "--compare-versions", aRelease, "lt", bRelease)
	err = cmd.Run()
	if err != nil {
		exitError, ok := err.(*exec.ExitError)
		if !ok {
			return false, err
		}
		if exitError.ExitCode() == 1 && string(exitError.Stderr) == "" {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
