package embedded

import (
	_ "embed"
	"fmt"

	"github.com/bigbag/papyrix-flasher/internal/protocol"
)

//go:embed bootloader.bin
var bootloaderC3 []byte

//go:embed bootloader-s3.bin
var bootloaderS3 []byte

//go:embed partitions.bin
var partitions []byte

// Bootloader returns the embedded bootloader for chipID.
func Bootloader(chipID uint32) ([]byte, error) {
	switch chipID {
	case protocol.ChipIDESP32C3:
		return bootloaderC3, nil
	case protocol.ChipIDESP32S3:
		return bootloaderS3, nil
	default:
		return nil, fmt.Errorf("unsupported bootloader chip ID: 0x%02X", chipID)
	}
}

// Partitions returns the embedded partition table binary.
func Partitions() []byte {
	return partitions
}
