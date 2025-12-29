package protocol

// ESP32 ROM bootloader commands
const (
	// Flash commands
	CmdFlashBegin    = 0x02
	CmdFlashData     = 0x03
	CmdFlashEnd      = 0x04
	CmdMemBegin      = 0x05
	CmdMemEnd        = 0x06
	CmdMemData       = 0x07
	CmdSync          = 0x08
	CmdWriteReg      = 0x09
	CmdReadReg       = 0x0A

	// SPI flash commands
	CmdSpiSetParams  = 0x0B
	CmdSpiAttach     = 0x0D
	CmdChangeBaud    = 0x0F
	CmdFlashDeflBegin = 0x10
	CmdFlashDeflData  = 0x11
	CmdFlashDeflEnd   = 0x12
	CmdSpiFlashMD5    = 0x13
	CmdGetSecurityInfo = 0x14

	// Stub-only commands (after stub is loaded)
	CmdEraseFlash    = 0xD0
	CmdEraseRegion   = 0xD1
	CmdReadFlash     = 0xD2
	CmdRunUserCode   = 0xD3
)

// Direction byte values
const (
	DirRequest  = 0x00
	DirResponse = 0x01
)

// Flash parameters
const (
	FlashBlockSize   = 0x400  // 1KB blocks
	FlashSectorSize  = 0x1000 // 4KB sectors
	FlashPageSize    = 0x100  // 256 byte pages

	// ESP32-C3 specific
	ESP32C3FlashFreq40M = 0x0F
	ESP32C3FlashModeDIO = 0x02
	ESP32C3FlashSize16MB = 0x40
)

// Chip IDs
const (
	ChipIDESP32   = 0x00
	ChipIDESP32S2 = 0x02
	ChipIDESP32C3 = 0x05
	ChipIDESP32S3 = 0x09
	ChipIDESP32C2 = 0x0C
	ChipIDESP32C6 = 0x0D
	ChipIDESP32H2 = 0x10
)

// ChipName returns human-readable name for chip ID
func ChipName(id uint32) string {
	switch id {
	case ChipIDESP32:
		return "ESP32"
	case ChipIDESP32S2:
		return "ESP32-S2"
	case ChipIDESP32C3:
		return "ESP32-C3"
	case ChipIDESP32S3:
		return "ESP32-S3"
	case ChipIDESP32C2:
		return "ESP32-C2"
	case ChipIDESP32C6:
		return "ESP32-C6"
	case ChipIDESP32H2:
		return "ESP32-H2"
	default:
		return "Unknown"
	}
}

// Error codes from ROM bootloader
const (
	ErrInvalidMessage   = 0x05
	ErrFailedToAct      = 0x06
	ErrInvalidCRC       = 0x07
	ErrFlashWriteErr    = 0x08
	ErrFlashReadErr     = 0x09
	ErrFlashReadLenErr  = 0x0A
	ErrDeflateError     = 0x0B
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
