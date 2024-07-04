package helper

import "github.com/snapcore/snapd/gadget"

// IsRootfsStructure determines if the given structure is the one associated
// to the rootfs
func IsRootfsStructure(s *gadget.VolumeStructure) bool {
	if s == nil {
		return false
	}
	return s.Role == gadget.SystemData
}

// IsSystemBootStructure determines if the given structure is the system-boot
// one
func IsSystemBootStructure(s *gadget.VolumeStructure) bool {
	if s == nil {
		return false
	}

	return s.Role == gadget.SystemBoot || s.Label == gadget.SystemBoot
}

// ShouldSkipStructure returns whether a structure should be skipped during certain processing
func ShouldSkipStructure(structure gadget.VolumeStructure, isSeeded bool) bool {
	if isSeeded &&
		(structure.Role == gadget.SystemBoot ||
			structure.Role == gadget.SystemData ||
			structure.Role == gadget.SystemSave ||
			structure.Label == gadget.SystemBoot) {
		return true
	}
	return false
}
