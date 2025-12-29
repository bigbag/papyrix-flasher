package protocol

// ESP32 ROM bootloader commands
const (
	CmdFlashEnd        = 0x04
	CmdSync            = 0x08
	CmdSpiSetParams    = 0x0B
	CmdSpiAttach       = 0x0D
	CmdFlashDeflBegin  = 0x10
	CmdFlashDeflData   = 0x11
	CmdFlashDeflEnd    = 0x12
	CmdGetSecurityInfo = 0x14
)

// Direction byte values
const (
	DirRequest  = 0x00
	DirResponse = 0x01
)

// Flash parameters
const (
	FlashBlockSize  = 0x400  // 1KB blocks
	FlashSectorSize = 0x1000 // 4KB sectors
)

// Chip IDs
const (
	ChipIDESP32C3 = 0x05
)

// ChipName returns human-readable name for chip ID
func ChipName(id uint32) string {
	switch id {
	case ChipIDESP32C3:
		return "ESP32-C3"
	default:
		return "ESP32"
	}
}

// Error codes from ROM bootloader
const (
	ErrInvalidMessage  = 0x05
	ErrFailedToAct     = 0x06
	ErrInvalidCRC      = 0x07
	ErrFlashWriteErr   = 0x08
	ErrFlashReadErr    = 0x09
	ErrFlashReadLenErr = 0x0A
	ErrDeflateError    = 0x0B
)

// ErrorMessage returns human-readable error message
func ErrorMessage(code byte) string {
	switch code {
	case ErrInvalidMessage:
		return "invalid message"
	case ErrFailedToAct:
		return "failed to act"
	case ErrInvalidCRC:
		return "invalid CRC"
	case ErrFlashWriteErr:
		return "flash write error"
	case ErrFlashReadErr:
		return "flash read error"
	case ErrFlashReadLenErr:
		return "flash read length error"
	case ErrDeflateError:
		return "deflate error"
	default:
		return "unknown error"
	}
}
