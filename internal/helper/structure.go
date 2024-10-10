package helper

import "github.com/snapcore/snapd/gadget"

// IsRootfsStructure determines if the given structure is the one associated
// to the rootfs
func IsRootfsStructure(s *gadget.VolumeStructure) bool {
	return IsSystemDataStructure(s)
}

// IsSystemBootStructure determines if the given structure is the system-boot
// one
func IsSystemBootStructure(s *gadget.VolumeStructure) bool {
	if s == nil {
		return false
	}

	return s.Role == gadget.SystemBoot || s.Label == gadget.SystemBoot
}

// IsSystemDataStructure determines if the given structure is the system-data
// one
func IsSystemDataStructure(s *gadget.VolumeStructure) bool {
	if s == nil {
		return false
	}

	return s.Role == gadget.SystemData
}

// IsSystemSeedStructure determines if the given structure is the system-seed
// one
func IsSystemSeedStructure(s *gadget.VolumeStructure) bool {
	if s == nil {
		return false
	}

	return s.Role == gadget.SystemSeed
}

// ShouldSkipStructure returns whether a structure should be skipped during certain processing
func ShouldSkipStructure(structure *gadget.VolumeStructure, isSeeded bool) bool {
	return isSeeded &&
		(structure.Role == gadget.SystemBoot ||
			structure.Label == gadget.SystemBoot ||
			structure.Role == gadget.SystemData ||
			structure.Role == gadget.SystemSave)
}
