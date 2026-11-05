package nodebootstrap

import (
	"errors"
	"regexp"
	"strconv"
)

// Stage 4 uses protocol-v1 agents from the 0.6.x release family only. A new
// wire protocol requires explicit review before widening this compatibility
// policy. All artifacts are independently signed before reaching this check.
var nodeReleasePattern = regexp.MustCompile(`^0\.6\.(0|[1-9][0-9]{0,5})(?:-[A-Za-z0-9_-]{1,32})?$`)

func versionPatch(version string) (int, error) {
	parts := nodeReleasePattern.FindStringSubmatch(version)
	if len(parts) != 2 {
		return 0, errors.New("node release version is outside the supported 0.6.x family")
	}
	patch, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, err
	}
	return patch, nil
}

// UpgradeAvailable is fail-closed for unknown, equal and older versions.
func UpgradeAvailable(installed, candidate string) error {
	current, err := versionPatch(installed)
	if err != nil {
		return err
	}
	next, err := versionPatch(candidate)
	if err != nil {
		return err
	}
	if next <= current {
		return errors.New("node upgrade must increase the signed patch version")
	}
	return nil
}
