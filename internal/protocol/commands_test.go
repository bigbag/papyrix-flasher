package protocol

import (
	"encoding/binary"
	"testing"
)

func TestChipName_KnownChips(t *testing.T) {
	tests := []struct {
		chipID   uint32
		expected string
	}{
		{ChipIDESP32C3, "ESP32-C3"},
	}

	for _, tc := range tests {
		result := ChipName(tc.chipID)
		if result != tc.expected {
			t.Errorf("ChipName(0x%X) = %q, want %q", tc.chipID, result, tc.expected)
		}
	}
}

func TestChipName_Unknown(t *testing.T) {
	unknownIDs := []uint32{0x00, 0x01, 0x99, 0xFFFFFFFF}
	for _, id := range unknownIDs {
		result := ChipName(id)
		if result != "ESP32" {
			t.Errorf("ChipName(0x%X) = %q, want %q", id, result, "ESP32")
		}
	}
}

func TestErrorMessage_AllCodes(t *testing.T) {
	tests := []struct {
		code     byte
		expected string
	}{
		{ErrInvalidMessage, "invalid message"},
		{ErrFailedToAct, "failed to act"},
		{ErrInvalidCRC, "invalid CRC"},
		{ErrFlashWriteErr, "flash write error"},
		{ErrFlashReadErr, "flash read error"},
		{ErrFlashReadLenErr, "flash read length error"},
		{ErrDeflateError, "deflate error"},
	}

	for _, tc := range tests {
		result := ErrorMessage(tc.code)
		if result != tc.expected {
			t.Errorf("ErrorMessage(0x%02X) = %q, want %q", tc.code, result, tc.expected)
		}
	}
}

func TestErrorMessage_Unknown(t *testing.T) {
	unknownCodes := []byte{0x00, 0x01, 0x04, 0xFF}
	for _, code := range unknownCodes {
		result := ErrorMessage(code)
		if result != "unknown error" {
			t.Errorf("ErrorMessage(0x%02X) = %q, want %q", code, result, "unknown error")
		}
	}
}

func TestSyncData(t *testing.T) {
	data := SyncData()

	// Should be 36 bytes
	if len(data) != 36 {
		t.Errorf("SyncData() length = %d, want 36", len(data))
	}

	// First 4 bytes are the sync pattern
	if data[0] != 0x07 || data[1] != 0x07 || data[2] != 0x12 || data[3] != 0x20 {
		t.Errorf("SyncData() header = %v, want [0x07, 0x07, 0x12, 0x20]", data[0:4])
	}

	// Remaining 32 bytes should be 0x55
	for i := 4; i < 36; i++ {
		if data[i] != 0x55 {
			t.Errorf("SyncData()[%d] = 0x%02X, want 0x55", i, data[i])
		}
	}
}

func TestFlashEndData_Reboot(t *testing.T) {
	data := FlashEndData(true)
	if len(data) != 4 {
		t.Errorf("FlashEndData(true) length = %d, want 4", len(data))
	}
	value := binary.LittleEndian.Uint32(data)
	if value != 0 {
		t.Errorf("FlashEndData(true) = %d, want 0", value)
	}
}

func TestFlashEndData_NoReboot(t *testing.T) {
	data := FlashEndData(false)
	if len(data) != 4 {
		t.Errorf("FlashEndData(false) length = %d, want 4", len(data))
	}
	value := binary.LittleEndian.Uint32(data)
	if value != 1 {
		t.Errorf("FlashEndData(false) = %d, want 1", value)
	}
}

func TestSpiAttachData(t *testing.T) {
	data := SpiAttachData()
	if len(data) != 8 {
		t.Errorf("SpiAttachData() length = %d, want 8", len(data))
	}
	for i, b := range data {
		if b != 0 {
			t.Errorf("SpiAttachData()[%d] = 0x%02X, want 0x00", i, b)
		}
	}
}

func TestSpiSetParamsData(t *testing.T) {
	totalSize := uint32(0x1000000) // 16MB
	data := SpiSetParamsData(totalSize)

	if len(data) != 24 {
		t.Errorf("SpiSetParamsData() length = %d, want 24", len(data))
	}

	// Check each field
	fields := []struct {
		offset   int
		expected uint32
		name     string
	}{
		{0, 0, "id"},
		{4, totalSize, "total size"},
		{8, 0x10000, "block size"},
		{12, 0x1000, "sector size"},
		{16, 0x100, "page size"},
		{20, 0xFFFF, "status mask"},
	}

	for _, f := range fields {
		value := binary.LittleEndian.Uint32(data[f.offset : f.offset+4])
		if value != f.expected {
			t.Errorf("SpiSetParamsData %s = 0x%X, want 0x%X", f.name, value, f.expected)
		}
	}
}

func TestFlashDeflBeginData(t *testing.T) {
	eraseSize := uint32(0x4000)
	numBlocks := uint32(4)
	blockSize := uint32(0x400)
	offset := uint32(0x10000)

	data := FlashDeflBeginData(eraseSize, numBlocks, blockSize, offset)

	if len(data) != 16 {
		t.Errorf("FlashDeflBeginData() length = %d, want 16", len(data))
	}

	fields := []struct {
		off      int
		expected uint32
		name     string
	}{
		{0, eraseSize, "erase size"},
		{4, numBlocks, "num blocks"},
		{8, blockSize, "block size"},
		{12, offset, "offset"},
	}

	for _, f := range fields {
		value := binary.LittleEndian.Uint32(data[f.off : f.off+4])
		if value != f.expected {
			t.Errorf("FlashDeflBeginData %s = 0x%X, want 0x%X", f.name, value, f.expected)
		}
	}
}

func TestFlashDeflDataData(t *testing.T) {
	compressedData := []byte{0x01, 0x02, 0x03, 0x04, 0x05}
	seq := uint32(7)

	data := FlashDeflDataData(compressedData, seq)

	expectedLen := 16 + len(compressedData)
	if len(data) != expectedLen {
		t.Errorf("FlashDeflDataData() length = %d, want %d", len(data), expectedLen)
	}

	// Check header fields
	dataLen := binary.LittleEndian.Uint32(data[0:4])
	if dataLen != uint32(len(compressedData)) {
		t.Errorf("FlashDeflDataData data length = %d, want %d", dataLen, len(compressedData))
	}

	seqNum := binary.LittleEndian.Uint32(data[4:8])
	if seqNum != seq {
		t.Errorf("FlashDeflDataData seq = %d, want %d", seqNum, seq)
	}

	// Reserved fields should be zero
	reserved1 := binary.LittleEndian.Uint32(data[8:12])
	reserved2 := binary.LittleEndian.Uint32(data[12:16])
	if reserved1 != 0 || reserved2 != 0 {
		t.Errorf("FlashDeflDataData reserved fields = (%d, %d), want (0, 0)", reserved1, reserved2)
	}

	// Check payload
	for i, b := range compressedData {
		if data[16+i] != b {
			t.Errorf("FlashDeflDataData payload[%d] = 0x%02X, want 0x%02X", i, data[16+i], b)
		}
	}
}

func TestFlashDeflEndData_Reboot(t *testing.T) {
	data := FlashDeflEndData(true)
	if len(data) != 4 {
		t.Errorf("FlashDeflEndData(true) length = %d, want 4", len(data))
	}
	value := binary.LittleEndian.Uint32(data)
	if value != 0 {
		t.Errorf("FlashDeflEndData(true) = %d, want 0", value)
	}
}

func TestFlashDeflEndData_NoReboot(t *testing.T) {
	data := FlashDeflEndData(false)
	if len(data) != 4 {
		t.Errorf("FlashDeflEndData(false) length = %d, want 4", len(data))
	}
	value := binary.LittleEndian.Uint32(data)
	if value != 1 {
		t.Errorf("FlashDeflEndData(false) = %d, want 1", value)
	}
}

func TestCalculateDeflBlocks_Exact(t *testing.T) {
	// Exact multiple of block size
	tests := []struct {
		compressedLen int
		blockSize     int
		expected      uint32
	}{
		{1024, 1024, 1},
		{2048, 1024, 2},
		{0, 1024, 0},
		{4096, 1024, 4},
	}

	for _, tc := range tests {
		result := CalculateDeflBlocks(tc.compressedLen, tc.blockSize)
		if result != tc.expected {
			t.Errorf("CalculateDeflBlocks(%d, %d) = %d, want %d",
				tc.compressedLen, tc.blockSize, result, tc.expected)
		}
	}
}

func TestCalculateDeflBlocks_Remainder(t *testing.T) {
	// Not exact multiple - should round up
	tests := []struct {
		compressedLen int
		blockSize     int
		expected      uint32
	}{
		{1, 1024, 1},
		{1025, 1024, 2},
		{2049, 1024, 3},
		{1023, 1024, 1},
	}

	for _, tc := range tests {
		result := CalculateDeflBlocks(tc.compressedLen, tc.blockSize)
		if result != tc.expected {
			t.Errorf("CalculateDeflBlocks(%d, %d) = %d, want %d",
				tc.compressedLen, tc.blockSize, result, tc.expected)
		}
	}
}

func TestCalculateEraseSize_Aligned(t *testing.T) {
	// Exact multiples of sector size (4KB)
	tests := []struct {
		dataLen  int
		expected uint32
	}{
		{0, 0},
		{4096, 4096},
		{8192, 8192},
		{16384, 16384},
	}

	for _, tc := range tests {
		result := CalculateEraseSize(tc.dataLen)
		if result != tc.expected {
			t.Errorf("CalculateEraseSize(%d) = %d, want %d", tc.dataLen, result, tc.expected)
		}
	}
}

func TestCalculateEraseSize_Unaligned(t *testing.T) {
	// Not exact multiples - should round up to next sector
	tests := []struct {
		dataLen  int
		expected uint32
	}{
		{1, 4096},
		{4095, 4096},
		{4097, 8192},
		{8193, 12288},
	}

	for _, tc := range tests {
		result := CalculateEraseSize(tc.dataLen)
		if result != tc.expected {
			t.Errorf("CalculateEraseSize(%d) = %d, want %d", tc.dataLen, result, tc.expected)
		}
	}
}

func TestParseSecurityInfo_Valid(t *testing.T) {
	data := make([]byte, 4)
	binary.LittleEndian.PutUint32(data, ChipIDESP32C3)

	info, err := ParseSecurityInfo(data)
	if err != nil {
		t.Fatalf("ParseSecurityInfo() error = %v", err)
	}
	if info.ChipID != ChipIDESP32C3 {
		t.Errorf("ParseSecurityInfo() ChipID = 0x%X, want 0x%X", info.ChipID, ChipIDESP32C3)
	}
}

func TestParseSecurityInfo_LongerData(t *testing.T) {
	// Real security info may have more data, we only read first 4 bytes
	data := make([]byte, 32)
	binary.LittleEndian.PutUint32(data, 0x12345678)

	info, err := ParseSecurityInfo(data)
	if err != nil {
		t.Fatalf("ParseSecurityInfo() error = %v", err)
	}
	if info.ChipID != 0x12345678 {
		t.Errorf("ParseSecurityInfo() ChipID = 0x%X, want 0x12345678", info.ChipID)
	}
}

func TestParseSecurityInfo_TooShort(t *testing.T) {
	shortData := []struct {
		data []byte
	}{
		{nil},
		{[]byte{}},
		{[]byte{0x01}},
		{[]byte{0x01, 0x02}},
		{[]byte{0x01, 0x02, 0x03}},
	}

	for _, tc := range shortData {
		_, err := ParseSecurityInfo(tc.data)
		if err == nil {
			t.Errorf("ParseSecurityInfo(%v) expected error, got nil", tc.data)
		}
	}
}

func TestConstants(t *testing.T) {
	// Verify command constants are correct
	commands := map[byte]string{
		CmdFlashEnd:        "CmdFlashEnd",
		CmdSync:            "CmdSync",
		CmdSpiSetParams:    "CmdSpiSetParams",
		CmdSpiAttach:       "CmdSpiAttach",
		CmdFlashDeflBegin:  "CmdFlashDeflBegin",
		CmdFlashDeflData:   "CmdFlashDeflData",
		CmdFlashDeflEnd:    "CmdFlashDeflEnd",
		CmdGetSecurityInfo: "CmdGetSecurityInfo",
	}

	expected := map[byte]byte{
		0x04: CmdFlashEnd,
		0x08: CmdSync,
		0x0B: CmdSpiSetParams,
		0x0D: CmdSpiAttach,
		0x10: CmdFlashDeflBegin,
		0x11: CmdFlashDeflData,
		0x12: CmdFlashDeflEnd,
		0x14: CmdGetSecurityInfo,
	}

	for val, cmd := range expected {
		if cmd != val {
			t.Errorf("%s = 0x%02X, want 0x%02X", commands[cmd], cmd, val)
		}
	}

	// Verify flash constants
	if FlashBlockSize != 0x400 {
		t.Errorf("FlashBlockSize = 0x%X, want 0x400", FlashBlockSize)
	}
	if FlashSectorSize != 0x1000 {
		t.Errorf("FlashSectorSize = 0x%X, want 0x1000", FlashSectorSize)
	}
}
