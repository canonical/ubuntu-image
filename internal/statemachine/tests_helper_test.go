package statemachine

import (
	"fmt"
	"io/fs"
)

type osMockConf struct {
	osutilCopySpecialFileThreshold uint
	ReadDirThreshold               uint
	RemoveThreshold                uint
	TruncateThreshold              uint
}

// osMock holds methods to easily mock functions from os and snapd/osutil packages
// Each method can be configured to fail after a given number of calls
// This could be improved by letting the mock functions calls the real
// functions before failing.
type osMock struct {
	conf                            *osMockConf
	beforeOsutilCopySpecialFileFail uint
	beforeReadDirFail               uint
	beforeRemoveFail                uint
	beforeTruncateFail              uint
}

func (o *osMock) CopySpecialFile(path, dest string) error {
	if o.beforeOsutilCopySpecialFileFail >= o.conf.osutilCopySpecialFileThreshold {
		return fmt.Errorf("CopySpecialFile fail")
	}
	o.beforeOsutilCopySpecialFileFail++

	return nil
}

func (o *osMock) ReadDir(name string) ([]fs.DirEntry, error) {
	if o.beforeReadDirFail >= o.conf.ReadDirThreshold {
		return nil, fmt.Errorf("ReadDir fail")
	}
	o.beforeReadDirFail++

	return []fs.DirEntry{}, nil
}

func (o *osMock) Remove(name string) error {
	if o.beforeRemoveFail >= o.conf.RemoveThreshold {
		return fmt.Errorf("Remove fail")
	}
	o.beforeRemoveFail++

	return nil
}

func (o *osMock) Truncate(name string, size int64) error {
	if o.beforeTruncateFail >= o.conf.TruncateThreshold {
		return fmt.Errorf("Truncate fail")
	}
	o.beforeTruncateFail++

	return nil
}

func NewOSMock(conf *osMockConf) *osMock {
	return &osMock{conf: conf}
}
