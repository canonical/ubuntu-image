package partition

import (
	"testing"

	"github.com/diskfs/go-diskfs/partition"
	"github.com/diskfs/go-diskfs/partition/gpt"
	"github.com/diskfs/go-diskfs/partition/mbr"
	"github.com/snapcore/snapd/gadget"
	"github.com/snapcore/snapd/gadget/quantity"

	"github.com/canonical/ubuntu-image/internal/helper"
)

// helper function to define *quantity.Offsets inline
func createOffsetPointer(x quantity.Offset) *quantity.Offset {
	return &x
}

var gadgetGPT = &gadget.Volume{
	Schema:     "gpt",
	Bootloader: "grub",
	Structure: []gadget.VolumeStructure{
		{
			VolumeName: "pc",
			Name:       "mbr",
			Offset:     createOffsetPointer(0),
			MinSize:    440,
			Size:       440,
			Type:       "mbr",
			Role:       "mbr",
			Content: []gadget.VolumeContent{
				{
					Image: "pc-boot.img",
				},
			},
			Update: gadget.VolumeUpdate{Edition: 1},
		},
		{
			VolumeName: "pc",
			Name:       "ubuntu-seed",
			Label:      "ubuntu-seed",
			Offset:     createOffsetPointer(1048576),
			MinSize:    1258291200,
			Size:       1258291200,
			Type:       "EF,C12A7328-F81F-11D2-BA4B-00A0C93EC93B",
			Role:       "system-seed",
			Filesystem: "vfat",
			Content:    []gadget.VolumeContent{},
			Update:     gadget.VolumeUpdate{Edition: 2},
			YamlIndex:  1,
		},
		{
			VolumeName: "pc",
			Name:       "",
			Label:      "writable",
			Offset:     createOffsetPointer(1259339776),
			MinSize:    1258291200,
			Size:       1258291200,
			Type:       "83,0FC63DAF-8483-4772-8E79-3D69D8477DE4",
			Role:       "system-data",
			Filesystem: "ext4",
			Content:    []gadget.VolumeContent{},
			YamlIndex:  2,
		},
	},
	Name: "pc",
}

var gadgetMBR = &gadget.Volume{
	Schema:     "mbr",
	Bootloader: "grub",
	Structure: []gadget.VolumeStructure{
		{
			VolumeName: "pc",
			Name:       "mbr",
			Offset:     createOffsetPointer(0),
			MinSize:    440,
			Size:       440,
			Type:       "mbr",
			Role:       "mbr",
			Content: []gadget.VolumeContent{
				{
					Image: "pc-boot.img",
				},
			},
			Update: gadget.VolumeUpdate{Edition: 1},
		},
		{
			VolumeName: "pc",
			Name:       "BIOS Boot",
			Offset:     createOffsetPointer(1048576),
			MinSize:    1258291200,
			Size:       1258291200,
			Type:       "DA",
			Content: []gadget.VolumeContent{
				{
					Image: "pc-core.img",
				},
			},
			Update:    gadget.VolumeUpdate{Edition: 2},
			YamlIndex: 1,
		},
	},
	Name: "pc",
}

var overlappingGadgetGPT = &gadget.Volume{
	Schema:     "gpt",
	Bootloader: "grub",
	Structure: []gadget.VolumeStructure{
		{
			VolumeName: "pc",
			Name:       "mbr",
			Offset:     createOffsetPointer(0),
			MinSize:    440,
			Size:       440,
			Type:       "mbr",
			Role:       "mbr",
			Content: []gadget.VolumeContent{
				{
					Image: "pc-boot.img",
				},
			},
			Update: gadget.VolumeUpdate{Edition: 1},
		},
		{
			VolumeName: "pc",
			Name:       "ubuntu-seed",
			Label:      "ubuntu-seed",
			Offset:     createOffsetPointer(1048576),
			MinSize:    1258291200,
			Size:       1258291200,
			Type:       "EF,C12A7328-F81F-11D2-BA4B-00A0C93EC93B",
			Role:       "system-seed",
			Filesystem: "vfat",
			Content:    []gadget.VolumeContent{},
			Update:     gadget.VolumeUpdate{Edition: 2},
			YamlIndex:  1,
		},
		{
			VolumeName: "pc",
			Name:       "writable",
			Label:      "writable",
			Offset:     createOffsetPointer(1), // should overlap first sectors
			MinSize:    1258291200,
			Size:       1258291200,
			Type:       "83,0FC63DAF-8483-4772-8E79-3D69D8477DE4",
			Role:       "system-data",
			Filesystem: "ext4",
			Content:    []gadget.VolumeContent{},
			YamlIndex:  2,
		},
	},
	Name: "pc",
}

func TestGeneratePartitionTable(t *testing.T) {
	type args struct {
		volume     *gadget.Volume
		sectorSize uint64
		imgSize    uint64
		isSeeded   bool
	}

	tests := []struct {
		name                 string
		args                 args
		wantPartitionTable   partition.Table
		wantRootfsPartNumber int
		expectedError        string
	}{
		{
			name: "GPT 512 sector size",
			args: args{
				volume:     gadgetGPT,
				sectorSize: sectorSize512,
				imgSize:    uint64(4 * quantity.SizeKiB),
			},
			wantRootfsPartNumber: 2,
			wantPartitionTable: &gpt.Table{
				LogicalSectorSize:  int(sectorSize512),
				PhysicalSectorSize: int(sectorSize512),
				ProtectiveMBR:      true,
				Partitions: []*gpt.Partition{
					{
						Start: 2048, // the Offset (1048576) divided by the sector size
						Size:  1258291200,
						Type:  "C12A7328-F81F-11D2-BA4B-00A0C93EC93B",
						Name:  "ubuntu-seed",
					},
					{
						Start: 2459648,
						Size:  1258291200,
						Type:  "0FC63DAF-8483-4772-8E79-3D69D8477DE4",
						Name:  "writable",
					},
				},
			},
		},
		{
			name: "GPT 4k sector size",
			args: args{
				volume:     gadgetGPT,
				sectorSize: sectorSize4k,
				imgSize:    uint64(4 * quantity.SizeKiB),
			},
			wantRootfsPartNumber: 2,
			wantPartitionTable: &gpt.Table{
				LogicalSectorSize:  int(sectorSize4k),
				PhysicalSectorSize: int(sectorSize4k),
				ProtectiveMBR:      true,
				Partitions: []*gpt.Partition{
					{
						Start: 256, // the Offset (1048576) divided by the sector size
						Size:  1258291200,
						Type:  "C12A7328-F81F-11D2-BA4B-00A0C93EC93B",
						Name:  "ubuntu-seed",
					},
					{
						Start: 307456,
						Size:  1258291200,
						Type:  "0FC63DAF-8483-4772-8E79-3D69D8477DE4",
						Name:  "writable",
					},
				},
			},
		},
		{
			name: "GPT 512 sector size",
			args: args{
				volume:     gadgetGPT,
				sectorSize: sectorSize512,
				imgSize:    uint64(4 * quantity.SizeKiB),
			},
			wantRootfsPartNumber: 2,
			wantPartitionTable: &gpt.Table{
				LogicalSectorSize:  int(sectorSize512),
				PhysicalSectorSize: int(sectorSize512),
				ProtectiveMBR:      true,
				Partitions: []*gpt.Partition{
					{
						Start: 2048, // the Offset (1048576) divided by the sector size
						Size:  1258291200,
						Type:  "C12A7328-F81F-11D2-BA4B-00A0C93EC93B",
						Name:  "ubuntu-seed",
					},
					{
						Start: 2459648,
						Size:  1258291200,
						Type:  "0FC63DAF-8483-4772-8E79-3D69D8477DE4",
						Name:  "writable",
					},
				},
			},
		},
		{
			name: "overlaping structures",
			args: args{
				volume:     overlappingGadgetGPT,
				sectorSize: sectorSize512,
				imgSize:    uint64(4 * quantity.SizeKiB),
			},
			wantRootfsPartNumber: 2,
			expectedError:        `The structure "writable" overlaps GPT header or GPT partition table`,
		},
		{
			name: "MBR 512 sector size",
			args: args{
				volume:     gadgetMBR,
				sectorSize: sectorSize512,
				imgSize:    uint64(4 * quantity.SizeKiB),
			},
			wantRootfsPartNumber: -1,
			wantPartitionTable: &mbr.Table{
				Partitions: []*mbr.Partition{
					{
						Type:  218,
						Start: 2048,
						Size:  2457600,
					},
				},
				LogicalSectorSize:  512,
				PhysicalSectorSize: 512,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			asserter := &helper.Asserter{T: t}
			gotPartitionTable, gotRootfsPartNumber, gotErr := GeneratePartitionTable(tt.args.volume, tt.args.sectorSize, tt.args.imgSize, tt.args.isSeeded)

			if len(tt.expectedError) == 0 {
				asserter.AssertErrNil(gotErr, true)
				asserter.AssertEqual(tt.wantRootfsPartNumber, gotRootfsPartNumber)
				asserter.AssertEqual(tt.wantPartitionTable, gotPartitionTable)
			} else {
				asserter.AssertErrContains(gotErr, tt.expectedError)
			}
		})
	}
}

func TestGPTTable_PartitionTableSize(t *testing.T) {
	type fields struct {
		concreteTable *gpt.Table
		diskSize      uint64
	}
	tests := []struct {
		name   string
		fields fields
		want   uint64
	}{
		{
			name: "512 sector size",
			fields: fields{
				concreteTable: &gpt.Table{
					LogicalSectorSize: int(sectorSize512),
				},
			},
			want: (1 + (1+32)*2) * sectorSize512,
		},
		{
			name: "4k sector size",
			fields: fields{
				concreteTable: &gpt.Table{
					LogicalSectorSize: int(sectorSize4k),
				},
			},
			want: (1 + (1+4)*2) * sectorSize4k,
		},
	}
	// See doc on structureOverlapsTable to understand the math of the resulting wanted size
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			asserter := &helper.Asserter{T: t}
			tr := &GPTTable{
				concreteTable: tc.fields.concreteTable,
				diskSize:      tc.fields.diskSize,
			}
			got := tr.PartitionTableSize()
			asserter.AssertEqual(tc.want, got)
		})
	}
}

func TestPartitionTableSizeFromVolume(t *testing.T) {
	type args struct {
		volume     *gadget.Volume
		sectorSize uint64
		imgSize    uint64
	}
	tests := []struct {
		name     string
		args     args
		wantSize uint64
	}{
		{
			name: "gpt 512 sector size",
			args: args{
				sectorSize: sectorSize512,
				volume:     gadgetGPT,
				imgSize:    uint64(4 * quantity.SizeKiB),
			},
			wantSize: (1 + (1+32)*2) * sectorSize512,
		},
		{
			name: "gpt 4k sector size",
			args: args{
				sectorSize: sectorSize4k,
				volume:     gadgetGPT,
				imgSize:    uint64(4 * quantity.SizeKiB),
			},
			wantSize: (1 + (1+4)*2) * sectorSize4k,
		},
		{
			name: "MBR 512 sector size",
			args: args{
				sectorSize: sectorSize512,
				volume:     gadgetMBR,
				imgSize:    uint64(4 * quantity.SizeKiB),
			},
			wantSize: sectorSize512,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			asserter := &helper.Asserter{T: t}
			gotSize := PartitionTableSizeFromVolume(tt.args.volume, tt.args.sectorSize, tt.args.imgSize)
			asserter.AssertEqual(tt.wantSize, gotSize)
		})
	}
}
