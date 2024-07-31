package partition

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/diskfs/go-diskfs/partition"
	"github.com/diskfs/go-diskfs/partition/gpt"
	"github.com/diskfs/go-diskfs/partition/mbr"
	"github.com/snapcore/snapd/gadget"

	"github.com/canonical/ubuntu-image/internal/helper"
)

const (
	// SchemaMBR identifies a Master Boot Record partitioning schema, or an
	// MBR like role
	SchemaMBR = "mbr"
	// SchemaGPT identifies a GUID Partition Table partitioning schema
	SchemaGPT = "gpt"

	sectorSize512 uint64 = 512
	sectorSize4k  uint64 = 4096

	protectiveMBRSectors   uint64 = 1
	partitionHeaderSectors uint64 = 1
)

// Table is a light wrapper around partition.Table to properly add partitions
// Some work is sadly duplicated because the go-diskfs lib does not expose
// the needed data (first/last LBA)
type Table interface {
	AddPartition(structurePair *gadget.OnDiskAndGadgetStructurePair, structureType string) error
	GetConcreteTable() partition.Table
	PartitionTableSize() uint64
}

// newPartitionTable creates a partition table for a given volume
func newPartitionTable(volume *gadget.Volume, sectorSize uint64, imgSize uint64) Table {
	if volume.Schema == SchemaMBR {
		return &MBRTable{
			diskSize: imgSize,
			concreteTable: &mbr.Table{
				LogicalSectorSize:  int(sectorSize),
				PhysicalSectorSize: int(sectorSize),
			},
		}
	}
	return &GPTTable{
		diskSize: imgSize,
		concreteTable: &gpt.Table{
			LogicalSectorSize:  int(sectorSize),
			PhysicalSectorSize: int(sectorSize),
			ProtectiveMBR:      true,
		},
	}
}

// GeneratePartitionTable prepares the partition table for structures in a volume and
// returns it with the partition number of the root partition.
func GeneratePartitionTable(volume *gadget.Volume, sectorSize uint64, imgSize uint64, isSeeded bool) (partition.Table, int, error) {
	partitionNumber, rootfsPartitionNumber := 1, -1
	partitionTable := newPartitionTable(volume, sectorSize, imgSize)
	onDisk := gadget.OnDiskStructsFromGadget(volume)

	for i := range volume.Structure {
		structure := &volume.Structure[i]
		if !structure.IsPartition() || helper.ShouldSkipStructure(structure, isSeeded) {
			continue
		}

		// Record the actual partition number of the root partition, as it
		// might be useful for certain operations (like updating the bootloader)
		if helper.IsRootfsStructure(structure) { //nolint:gosec,G301
			rootfsPartitionNumber = partitionNumber
		}

		structurePair := &gadget.OnDiskAndGadgetStructurePair{
			DiskStructure:   onDisk[structure.YamlIndex],
			GadgetStructure: &volume.Structure[i],
		}

		structureType := getStructureType(structure, volume.Schema)
		err := partitionTable.AddPartition(structurePair, structureType)
		if err != nil {
			return nil, rootfsPartitionNumber, err
		}

		partitionNumber++
	}

	return partitionTable.GetConcreteTable(), rootfsPartitionNumber, nil
}

// getStructureType extracts the structure type from the structure.Type considering
// the schema
func getStructureType(structure *gadget.VolumeStructure, schema string) string {
	structureType := structure.Type
	// Check for hybrid MBR/GPT
	if !strings.Contains(structure.Type, ",") {
		return structureType
	}

	types := strings.Split(structure.Type, ",")
	structureType = types[0]

	if schema == SchemaGPT {
		structureType = types[1]
	}

	return structureType
}

// PartitionTableSizeFromVolume returns the total size in bytes of the partition table
func PartitionTableSizeFromVolume(volume *gadget.Volume, sectorSize uint64, imgSize uint64) uint64 {
	t := newPartitionTable(volume, sectorSize, imgSize)
	return t.PartitionTableSize()
}

type MBRTable struct {
	concreteTable *mbr.Table
	diskSize      uint64
}

func (t *MBRTable) AddPartition(structurePair *gadget.OnDiskAndGadgetStructurePair, structureType string) error {
	// mbr.Type is a byte. snapd has already verified that this string
	// is exactly two chars, so we can safely parse those two chars to a byte
	partitionType, _ := strconv.ParseUint(structureType, 16, 8) // nolint: errcheck

	t.concreteTable.Partitions = append(t.concreteTable.Partitions, &mbr.Partition{
		Start:    uint32(math.Ceil(float64(structurePair.DiskStructure.StartOffset) / float64(t.concreteTable.LogicalSectorSize))),
		Size:     uint32(math.Ceil(float64(structurePair.DiskStructure.Size) / float64(t.concreteTable.LogicalSectorSize))),
		Type:     mbr.Type(partitionType),
		Bootable: helper.IsSystemBootStructure(structurePair.GadgetStructure),
	})

	return nil
}

func (t *MBRTable) GetConcreteTable() partition.Table {
	return t.concreteTable
}

// PartitionTableSize returns the total size in bytes of the partition table
func (t *MBRTable) PartitionTableSize() uint64 {
	return uint64(t.concreteTable.LogicalSectorSize)
}

type GPTTable struct {
	concreteTable *gpt.Table
	diskSize      uint64
}

func (t *GPTTable) AddPartition(structurePair *gadget.OnDiskAndGadgetStructurePair, structureType string) error {
	startSector := uint64(math.Ceil(float64(structurePair.DiskStructure.StartOffset) / float64(t.concreteTable.LogicalSectorSize)))
	size := uint64(structurePair.DiskStructure.Size)
	partitionName := structurePair.DiskStructure.Name

	if t.structureOverlapsTable(startSector, size) {
		return fmt.Errorf("The structure \"%s\" overlaps GPT header or "+
			"GPT partition table", partitionName)
	}

	if helper.IsSystemDataStructure(structurePair.GadgetStructure) && partitionName == "" {
		partitionName = "writable"
	}

	t.concreteTable.Partitions = append(t.concreteTable.Partitions, &gpt.Partition{
		Start: startSector,
		Size:  size,
		Type:  gpt.Type(structureType),
		Name:  partitionName,
	})

	return nil
}

func (t *GPTTable) GetConcreteTable() partition.Table {
	return t.concreteTable
}

func gptPartitionEntriesSectors(sectorSize uint64) uint64 {
	var partitionEntriesSectors uint64 = 32

	if sectorSize == sectorSize4k {
		partitionEntriesSectors = 4
	}

	return partitionEntriesSectors
}

// primaryGPTSectors returns how many sectors the primary GPT header uses
func (t *GPTTable) primaryGPTSectors() uint64 {
	return protectiveMBRSectors + partitionHeaderSectors + gptPartitionEntriesSectors(uint64(t.concreteTable.LogicalSectorSize))
}

// primaryGPTSectors returns how many sectors the secondary GPT header uses
func (t *GPTTable) secondaryGPTSectors() uint64 {
	return partitionHeaderSectors + gptPartitionEntriesSectors(uint64(t.concreteTable.LogicalSectorSize))
}

func (t *GPTTable) sizeToSectors(size uint64) uint64 {
	return uint64(math.Ceil(float64(size) / float64(t.concreteTable.LogicalSectorSize)))
}

// structureOverlapsTable checks if a given structure overlaps the GPT table (either the primary
// or secondary one)
// If the block size is 512, the First Usable LBA must be greater than or equal
// to 34 (allowing 1 block for the Protective MBR, 1 block for the Partition
// Table Header, and 32 blocks for the GPT Partition Entry Array)
// If the logical block size is 4096, the First Useable LBA must be greater than
// or equal to 6 (allowing 1 block for the Protective MBR, 1 block for the GPT
// Header, and 4 blocks for the GPT Partition Entry Array)
func (t *GPTTable) structureOverlapsTable(startSector uint64, size uint64) bool {
	diskSectors := t.sizeToSectors(t.diskSize)
	structureSectors := t.sizeToSectors(size)

	return startSector < t.primaryGPTSectors() || startSector+structureSectors > diskSectors-t.secondaryGPTSectors()
}

// PartitionTableSize returns the total size in bytes of the partition table
func (t *GPTTable) PartitionTableSize() uint64 {
	return (t.primaryGPTSectors() + t.secondaryGPTSectors()) * uint64(t.concreteTable.LogicalSectorSize)
}
