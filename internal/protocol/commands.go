package protocol

// ESP32 ROM bootloader commands
const (
	CmdFlashEnd        = 0x04
	CmdMemBegin        = 0x05
	CmdMemEnd          = 0x06
	CmdMemData         = 0x07
	CmdSync            = 0x08
	CmdWriteReg        = 0x09
	CmdReadReg         = 0x0A
	CmdSpiSetParams    = 0x0B
	CmdSpiAttach       = 0x0D
	CmdChangeBaudrate  = 0x0F
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
	FlashBlockSize     = 0x400  // 1KB blocks (ROM bootloader)
	FlashSectorSize    = 0x1000 // 4KB sectors
	StubFlashWriteSize = 0x4000 // 16KB blocks when stub is running
	MemBlockSize       = 0x1800 // 6KB blocks for RAM upload
)

// Chip IDs
const (
	ChipIDESP32C3 = 0x05
	ChipIDESP32S3 = 0x09
)

// Response status trailer sizes. ROM bootloaders append four bytes
// (status, error, two reserved); the stub flasher appends only status and error.
const (
	ROMStatusBytes  = 4
	StubStatusBytes = 2
)

// ChipName returns human-readable name for chip ID
func ChipName(id uint32) string {
	switch id {
	case ChipIDESP32C3:
		return "ESP32-C3"
	case ChipIDESP32S3:
		return "ESP32-S3"
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

// ESP32-C3 register addresses for watchdog control (USB-JTAG/Serial)
const (
	UartdevBufNo          = 0x3FCDF07C // ROM .bss variable indicating active port
	UartdevBufNoUSBJTAG   = 3          // Value when USB-JTAG/Serial is active
	RTCCntlBaseReg        = 0x60008000
	RTCCntlWdtConfig0Reg  = RTCCntlBaseReg + 0x0090
	RTCCntlWdtWprotectReg = RTCCntlBaseReg + 0x00A8
	RTCCntlWdtWkey        = 0x50D83AA1
	RTCCntlSwdConfReg     = RTCCntlBaseReg + 0x00AC
	RTCCntlSwdAutoFeedEn  = 1 << 31
	RTCCntlSwdWprotectReg = RTCCntlBaseReg + 0x00B0
	RTCCntlSwdWkey        = 0x8F1D312A
)
