package statemachine

import (
	"fmt"
	"os/exec"
	"slices"
	"strings"

	"github.com/canonical/ubuntu-image/internal/helper"
)

// isSeriesEqualOrOlder returns true if a is equal to or older than b
func isSeriesEqualOrOlder(a, b string) (bool, error) {
	if a == b {
		return true, nil
	}
	// ubuntu-distro-info --all returns series sorted by release date
	cmd := exec.Command("ubuntu-distro-info", "--all")
	outputBytes, err := cmd.Output()
	if err != nil {
		return true, helper.WrapCmdError(cmd, err)
	}
	seriesList := strings.Split(string(outputBytes), "\n")

	aPosition := slices.Index(seriesList, a)
	bPosition := slices.Index(seriesList, b)

	for name, pos := range map[string]int{a: aPosition, b: bPosition} {
		if pos == -1 {
			return true, fmt.Errorf("unknown series: %s", name)
		}
	}

	return aPosition <= bPosition, nil
}
