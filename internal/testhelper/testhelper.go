// Package testhelper provides helpers to ease mocking functions and methods
// provided by packages such as os or http.
package testhelper

import (
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
)

// SaveCWD gets the current working directory and returns a function to go back to it
// nolint: errcheck
func SaveCWD() func() {
	wd, _ := os.Getwd()
	return func() {
		_ = os.Chdir(wd)
	}
}

// OSMockConf enables setting thresholds to indicate how many calls a the mocked
// functions should accept before returning an error.
// See osMock methods for specific behaviors.
type OSMockConf struct {
	OsutilCopySpecialFileThreshold uint
	ReadDirThreshold               uint
	RemoveThreshold                uint
	RemoveAllThreshold             uint
	TruncateThreshold              uint
	OpenFileThreshold              uint
	MkdirAllThreshold              uint
	HttpGetThreshold               uint
	ReadAllThreshold               uint
}

// osMock holds methods to easily mock functions from os and snapd/osutil packages
// Each method can be configured to fail after a given number of calls
// This could be improved by letting the mock functions calls the real
// functions before failing.
type osMock struct {
	conf                            *OSMockConf
	beforeOsutilCopySpecialFileFail uint
	beforeReadDirFail               uint
	beforeRemoveFail                uint
	beforeRemoveAllFail             uint
	beforeTruncateFail              uint
	beforeOpenFileFail              uint
	beforeMkdirAllFail              uint
	beforeHttpGetFail               uint
	beforeReadAllFail               uint
}

// CopySpecialFile mocks CopySpecialFile github.com/snapcore/snapd/osutil
func (o *osMock) CopySpecialFile(path, dest string) error {
	if o.beforeOsutilCopySpecialFileFail >= o.conf.OsutilCopySpecialFileThreshold {
		return fmt.Errorf("CopySpecialFile fail")
	}
	o.beforeOsutilCopySpecialFileFail++

	return nil
}

// ReadDir mocks os.ReadDir
func (o *osMock) ReadDir(name string) ([]fs.DirEntry, error) {
	if o.beforeReadDirFail >= o.conf.ReadDirThreshold {
		return nil, fmt.Errorf("ReadDir fail")
	}
	o.beforeReadDirFail++

	return []fs.DirEntry{}, nil
}

// Remove mocks os.Remove
func (o *osMock) Remove(name string) error {
	if o.beforeRemoveFail >= o.conf.RemoveThreshold {
		return fmt.Errorf("Remove fail")
	}
	o.beforeRemoveFail++

	return nil
}

func (o *osMock) RemoveAll(name string) error {
	if o.beforeRemoveAllFail >= o.conf.RemoveAllThreshold {
		return fmt.Errorf("RemoveAll fail")
	}
	o.beforeRemoveAllFail++

	return nil
}

// Truncate mocks osTruncate
func (o *osMock) Truncate(name string, size int64) error {
	if o.beforeTruncateFail >= o.conf.TruncateThreshold {
		return fmt.Errorf("Truncate fail")
	}
	o.beforeTruncateFail++

	return nil
}

// OpenFile mocks os.OpenFile
func (o *osMock) OpenFile(name string, flag int, perm os.FileMode) (*os.File, error) {
	if o.beforeOpenFileFail >= o.conf.OpenFileThreshold {
		return nil, fmt.Errorf("OpenFile fail")
	}
	o.beforeOpenFileFail++

	return &os.File{}, nil
}

// MkdirAll mocks os.MkdirAll
func (o *osMock) MkdirAll(path string, perm os.FileMode) error {
	if o.beforeOpenFileFail >= o.conf.OpenFileThreshold {
		return fmt.Errorf("OpenFile fail")
	}
	o.beforeMkdirAllFail++

	return nil
}

// HttpGet mocks http.Get
func (o *osMock) HttpGet(path string) (*http.Response, error) {
	if o.beforeHttpGetFail >= o.conf.HttpGetThreshold {
		return nil, fmt.Errorf("HttpGet fail")
	}
	o.beforeHttpGetFail++

	return &http.Response{}, nil
}

// ReadAll mocks os.ReadAll
func (o *osMock) ReadAll(io.Reader) ([]byte, error) {
	if o.beforeReadAllFail >= o.conf.ReadAllThreshold {
		return nil, fmt.Errorf("ReadAll fail")
	}
	o.beforeReadAllFail++

	return []byte{}, nil
}

func NewOSMock(conf *OSMockConf) *osMock {
	return &osMock{conf: conf}
}

type SfdiskOutput struct {
	PartitionTable PartitionTable
}

type PartitionTable struct {
	Label      string
	Id         string
	Device     string
	Unit       string
	FirstLBA   uint64
	LastLBA    uint64
	SectorSize uint64
	Partitions []SfDiskPartitions
}

type SfDiskPartitions struct {
	Node  string
	Start uint64
	Size  uint64
	Type  string
	Uuid  string
	Name  string
}
