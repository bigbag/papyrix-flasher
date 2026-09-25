package protocol

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

const (
	imageMagic       = 0xE9
	appPartitionSize = 0x640000
	// ESP-IDF application descriptor magic word, first word of the first
	// (DROM) segment of an application image.
	appDescriptorMagic = 0xABCD5432
)

// ValidateApplication checks that data is a well-formed ESP application image
// for a supported chip and returns its chip ID. It verifies the image header,
// segment bounds, XOR checksum, optional SHA-256 digest, and the presence of
// the ESP-IDF application descriptor. It does not read the device and does
// not establish authenticity.
func ValidateApplication(data []byte) (uint32, error) {
	const headerSize = 24

	if len(data) < headerSize {
		return 0, fmt.Errorf("image too short: %d bytes", len(data))
	}
	if data[0] != imageMagic {
		return 0, fmt.Errorf("invalid image magic: 0x%02X", data[0])
	}
	chipID := binary.LittleEndian.Uint16(data[12:14])
	if chipID != ChipIDESP32C3 && chipID != ChipIDESP32S3 {
		return 0, fmt.Errorf("unsupported image chip ID: 0x%02X (supported: 0x%02X ESP32-C3, 0x%02X ESP32-S3)",
			chipID, ChipIDESP32C3, ChipIDESP32S3)
	}
	segments := int(data[1])
	if segments < 1 || segments > 16 {
		return 0, fmt.Errorf("invalid segment count: %d", segments)
	}
	appendDigest := data[23]
	if appendDigest > 1 {
		return 0, fmt.Errorf("invalid digest flag: 0x%02X", appendDigest)
	}
	if len(data) > appPartitionSize {
		return 0, fmt.Errorf("image exceeds app0 partition size 0x%X: %d bytes", appPartitionSize, len(data))
	}

	// The first segment must start with the ESP-IDF application descriptor.
	cursor := headerSize
	checksum := byte(0xEF)
	for i := range segments {
		if cursor+8 > len(data) {
			return 0, fmt.Errorf("segment %d header out of bounds", i)
		}
		size := binary.LittleEndian.Uint32(data[cursor+4 : cursor+8])
		remain, err := Uint32Len(len(data) - cursor - 8)
		if err != nil || size > remain {
			return 0, fmt.Errorf("segment %d length %d runs past end of image", i, size)
		}
		segStart := cursor + 8
		segEnd := segStart + int(size)
		for _, b := range data[segStart:segEnd] {
			checksum ^= b
		}
		if i == 0 {
			if size < 4 || binary.LittleEndian.Uint32(data[segStart:segStart+4]) != appDescriptorMagic {
				return 0, fmt.Errorf("first segment lacks ESP-IDF application descriptor (not an application image?)")
			}
		}
		cursor = segEnd
	}

	// Checksum byte is the last byte of the 16-byte block after the segments.
	checksumOffset := ((cursor+16)/16)*16 - 1
	if checksumOffset >= len(data) {
		return 0, fmt.Errorf("checksum byte out of bounds")
	}
	if data[checksumOffset] != checksum {
		return 0, fmt.Errorf("checksum mismatch: image byte 0x%02X, computed 0x%02X", data[checksumOffset], checksum)
	}
	end := checksumOffset + 1

	if appendDigest == 1 {
		if end+32 > len(data) {
			return 0, fmt.Errorf("appended digest truncated")
		}
		digest := sha256.Sum256(data[:end])
		if !bytes.Equal(digest[:], data[end:end+32]) {
			return 0, fmt.Errorf("image digest mismatch")
		}
		end += 32
	}

	for _, b := range data[end:] {
		if b != 0xFF {
			return 0, fmt.Errorf("unexpected data after image end at %d", end)
		}
	}

	return uint32(chipID), nil
}
