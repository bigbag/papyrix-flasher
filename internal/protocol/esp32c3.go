package protocol

import (
	"encoding/binary"
	"fmt"
)

// ESP32C3Info contains information about an ESP32-C3 chip.
type ESP32C3Info struct {
	ChipID    uint32
	Revision  uint8
	Features  uint32
	MAC       [6]byte
}

// ParseSecurityInfo parses the response from GET_SECURITY_INFO command.
func ParseSecurityInfo(data []byte) (*ESP32C3Info, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("security info too short: %d bytes", len(data))
	}

	info := &ESP32C3Info{}

	// The chip ID is encoded in the security info response
	// Format varies by chip, but chip_id is typically in first 4 bytes
	info.ChipID = binary.LittleEndian.Uint32(data[0:4])

	return info, nil
}

// ReadMACFromEfuse reads the MAC address from eFuse data.
// This is a placeholder - actual MAC reading requires eFuse register access.
func ReadMACFromEfuse(data []byte) [6]byte {
	var mac [6]byte
	if len(data) >= 6 {
		copy(mac[:], data[:6])
	}
	return mac
}

// ESP32C3 flash configuration constants
const (
	// Flash addresses for Papyrix
	BootloaderAddress = 0x0000
	PartitionsAddress = 0x8000
	FirmwareAddress   = 0x10000

	// Default baud rate
	DefaultBaudRate = 921600

	// ESP32-C3 stub loader entry point
	StubEntryAddress = 0x4004A000
)

// CalculateFlashBlocks calculates the number of blocks needed for a given size.
func CalculateFlashBlocks(size int) uint32 {
	blocks := size / FlashBlockSize
	if size%FlashBlockSize != 0 {
		blocks++
	}
	return uint32(blocks)
}

// CalculateEraseSize calculates the erase size needed (sector-aligned).
func CalculateEraseSize(size int) uint32 {
	sectors := size / FlashSectorSize
	if size%FlashSectorSize != 0 {
		sectors++
	}
	return uint32(sectors * FlashSectorSize)
}
