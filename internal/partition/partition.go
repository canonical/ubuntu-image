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
)

// Table is light wrapper around partition.Table to properly add partitions
// Some work is sadly duplicated because the go-diskfs lib does not expose
// the needed data (first/last LBA)
type Table interface {
	AddPartition(structure gadget.VolumeStructure, structureType string) error
	GetConcreteTable() partition.Table
}

// NewPartitionTable creates a partition table for a given volume
func NewPartitionTable(volume *gadget.Volume, sectorSize uint64, imgSize uint64) Table {
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

// GeneratePartitionTable prepares the partition table for a structures in a volume and
// returns it with the partition number of the root partition.
func GeneratePartitionTable(volume *gadget.Volume, sectorSize uint64, imgSize uint64, isSeeded bool) (partition.Table, int, error) {
	partitionNumber, rootfsPartitionNumber := 1, -1

	partitionTable := NewPartitionTable(volume, sectorSize, imgSize)

	for _, structure := range volume.Structure {
		if !structure.IsPartition() || helper.ShouldSkipStructure(structure, isSeeded) {
			continue
		}

		// Record the actual partition number of the root partition, as it
		// might be useful for certain operations (like updating the bootloader)
		if helper.IsRootfsStructure(&structure) { //nolint:gosec,G301
			rootfsPartitionNumber = partitionNumber
		}

		structureType := getStructureType(structure, volume.Schema)

		err := partitionTable.AddPartition(structure, structureType)
		if err != nil {
			return nil, rootfsPartitionNumber, err
		}

		partitionNumber++
	}

	return partitionTable.GetConcreteTable(), rootfsPartitionNumber, nil
}

// getStructureType extracts the structure type from the structure.Type considering
// the schema
func getStructureType(structure gadget.VolumeStructure, schema string) string {
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

type MBRTable struct {
	concreteTable *mbr.Table
	diskSize      uint64
}

func (t *MBRTable) AddPartition(structure gadget.VolumeStructure, structureType string) error {
	// mbr.Type is a byte. snapd has already verified that this string
	// is exactly two chars, so we can safely parse those two chars to a byte
	partitionType, _ := strconv.ParseUint(structureType, 16, 8) // nolint: errcheck

	t.concreteTable.Partitions = append(t.concreteTable.Partitions, &mbr.Partition{
		Start:    uint32(math.Ceil(float64(*structure.Offset) / float64(t.concreteTable.LogicalSectorSize))),
		Size:     uint32(math.Ceil(float64(structure.Size) / float64(t.concreteTable.LogicalSectorSize))),
		Type:     mbr.Type(partitionType),
		Bootable: helper.IsSystemBootStructure(&structure),
	})

	return nil
}

func (t *MBRTable) GetConcreteTable() partition.Table {
	return t.concreteTable
}

type GPTTable struct {
	concreteTable *gpt.Table
	diskSize      uint64
}

func (t *GPTTable) AddPartition(structure gadget.VolumeStructure, structureType string) error {
	startSector := uint64(math.Ceil(float64(*structure.Offset) / float64(t.concreteTable.LogicalSectorSize)))
	size := uint64(structure.Size)

	if t.structureOverlaps(startSector, size) {
		return fmt.Errorf("The structure \"%s\" overlaps GPT header or "+
			"GPT partition table", structure.Name)
	}

	partitionName := structure.Name
	if structure.Role == gadget.SystemData && structure.Name == "" {
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

// structureOverlaps checks if a given structure overlaps the GPT table (either the primary
// or secondary one)
// If the block size is 512, the First Usable LBA must be greater than or equal
// to 34 (allowing 1 block for the Protective MBR, 1 block for the Partition
// Table Header, and 32 blocks for the GPT Partition Entry Array)
// If the logical block size is 4096, the First Useable LBA must be greater than
// or equal to 6 (allowing 1 block for the Protective MBR, 1 block for the GPT
// Header, and 4 blocks for the GPT Partition Entry Array)
func (t *GPTTable) structureOverlaps(startSector uint64, size uint64) bool {
	var protectiveMBRSectors uint64 = 1
	var partitionHeaderSectors uint64 = 1
	var partitionEntriesSectors uint64 = 32

	if t.concreteTable.LogicalSectorSize == int(sectorSize4k) {
		partitionEntriesSectors = 4
	}

	var primaryGPTSectors uint64 = protectiveMBRSectors + partitionHeaderSectors + partitionEntriesSectors
	var secondaryGPTSectors uint64 = partitionHeaderSectors + partitionEntriesSectors

	diskSectors := uint64(math.Ceil(float64(t.diskSize) / float64(t.concreteTable.LogicalSectorSize)))
	structureSectors := uint64(math.Ceil(float64(size) / float64(t.concreteTable.LogicalSectorSize)))

	return startSector < primaryGPTSectors || startSector+structureSectors > diskSectors-secondaryGPTSectors
}
