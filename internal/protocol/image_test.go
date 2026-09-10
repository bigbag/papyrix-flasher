package protocol

import (
	"crypto/sha256"
	"encoding/binary"
	"testing"
)

// buildAppImage builds a minimal valid ESP application image:
// 24-byte header, one segment whose data starts with the ESP-IDF app
// descriptor magic, 16-byte-aligned checksum byte, and an optional SHA-256.
func buildAppImage(chipID uint16, withDigest bool) []byte {
	segmentData := make([]byte, 16)
	binary.LittleEndian.PutUint32(segmentData[0:4], 0xABCD5432)

	checksum := byte(0xEF)
	for _, b := range segmentData {
		checksum ^= b
	}

	img := make([]byte, 24)
	img[0] = 0xE9
	img[1] = 1 // segment count
	binary.LittleEndian.PutUint16(img[12:14], chipID)
	if withDigest {
		img[23] = 1
	}

	var segHeader [8]byte
	binary.LittleEndian.PutUint32(segHeader[0:4], 0x3C000000) // DROM load address
	binary.LittleEndian.PutUint32(segHeader[4:8], uint32(len(segmentData)))
	img = append(img, segHeader[:]...)
	img = append(img, segmentData...)

	end := ((len(img) + 16) / 16) * 16 // checksum sits in the last byte of this block
	for len(img) < end {
		img = append(img, 0xFF)
	}
	img[end-1] = checksum

	if withDigest {
		digest := sha256.Sum256(img)
		img = append(img, digest[:]...)
	}
	return img
}

func corruptByte(img []byte, offset int, value byte) []byte {
	out := make([]byte, len(img))
	copy(out, img)
	out[offset] = value
	return out
}

func TestValidateApplication_ValidC3(t *testing.T) {
	chip, err := ValidateApplication(buildAppImage(ChipIDESP32C3, true))
	if err != nil {
		t.Fatalf("ValidateApplication() error = %v", err)
	}
	if chip != ChipIDESP32C3 {
		t.Errorf("ValidateApplication() chip = 0x%X, want 0x%X", chip, ChipIDESP32C3)
	}
}

func TestValidateApplication_ValidS3WithoutDigest(t *testing.T) {
	chip, err := ValidateApplication(buildAppImage(ChipIDESP32S3, false))
	if err != nil {
		t.Fatalf("ValidateApplication() error = %v", err)
	}
	if chip != ChipIDESP32S3 {
		t.Errorf("ValidateApplication() chip = 0x%X, want 0x%X", chip, ChipIDESP32S3)
	}
}

func TestValidateApplication_RejectsInvalid(t *testing.T) {
	valid := buildAppImage(ChipIDESP32C3, true)

	oversized := make([]byte, 0x640001)
	copy(oversized, valid)

	badDigest := buildAppImage(ChipIDESP32C3, true)
	badDigest[len(badDigest)-1] ^= 0xFF

	// Keep the image otherwise valid so only descriptor validation rejects it.
	noDescriptor := buildAppImage(ChipIDESP32C3, false)
	binary.LittleEndian.PutUint32(noDescriptor[32:36], 0xDEADBEEF)
	checksum := byte(0xEF)
	for _, b := range noDescriptor[32:48] {
		checksum ^= b
	}
	noDescriptor[63] = checksum

	// Segment length runs past the end of the file.
	badSegLen := make([]byte, len(valid))
	copy(badSegLen, valid)
	binary.LittleEndian.PutUint32(badSegLen[28:32], 0x00FFFFFF)

	tests := []struct {
		name string
		data []byte
	}{
		{"nil", nil},
		{"header only", valid[:23]},
		{"bad magic", corruptByte(valid, 0, 0x00)},
		{"unsupported chip", corruptByte(valid, 12, 0x02)},
		{"zero segments", corruptByte(valid, 1, 0)},
		{"too many segments", corruptByte(valid, 1, 17)},
		{"bad digest flag", corruptByte(valid, 23, 2)},
		{"oversized", oversized},
		{"bad checksum", corruptByte(valid, 63, 0x00)},
		{"invalid digest", badDigest},
		{"no app descriptor", noDescriptor},
		{"segment length past EOF", badSegLen},
		{"trailing garbage", append(append([]byte{}, valid...), 0x12)},
	}

	for _, tc := range tests {
		_, err := ValidateApplication(tc.data)
		if err == nil {
			t.Errorf("ValidateApplication(%s) expected error, got nil", tc.name)
		}
	}
}

func TestValidateApplication_RejectsWrongChecksum(t *testing.T) {
	img := buildAppImage(ChipIDESP32S3, false)
	// Flip a payload bit without fixing the checksum.
	img[40] ^= 0x01

	if _, err := ValidateApplication(img); err == nil {
		t.Fatal("ValidateApplication() accepted image with wrong checksum")
	}
}
