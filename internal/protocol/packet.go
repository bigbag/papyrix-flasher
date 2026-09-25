package protocol

import (
	"encoding/binary"
	"fmt"
	"math"
)

// Request represents an ESP32 bootloader request packet.
type Request struct {
	Command  byte
	Data     []byte
	Checksum uint32
}

// Response represents an ESP32 bootloader response packet.
type Response struct {
	Command byte
	Data    []byte
	Value   uint32
	Status  byte
	Error   byte
}

// NewRequest creates a new request with checksum=0 (default for non-data commands).
func NewRequest(cmd byte, data []byte) *Request {
	return &Request{
		Command:  cmd,
		Data:     data,
		Checksum: 0,
	}
}

// NewDataRequest creates a request for data transfer commands (MEM_DATA,
// FLASH_DATA, FLASH_DEFL_DATA). The checksum covers only the raw payload
// (matching esptool), not the 16-byte header prepended by the *Data functions.
func NewDataRequest(cmd byte, data []byte, payload []byte) *Request {
	return &Request{
		Command:  cmd,
		Data:     data,
		Checksum: xorChecksum(payload),
	}
}

// xorChecksum computes the ESP32 bootloader checksum (XOR with magic 0xEF).
func xorChecksum(data []byte) uint32 {
	var checksum byte = 0xEF
	for _, b := range data {
		checksum ^= b
	}
	return uint32(checksum)
}

// Uint32Len converts n to uint32.
// It returns an error when n is outside the uint32 range.
func Uint32Len(n int) (uint32, error) {
	if n < 0 {
		return 0, fmt.Errorf("value %d is outside the uint32 range", n)
	}
	if n > math.MaxUint32 {
		return 0, fmt.Errorf("value %d is outside the uint32 range", n)
	}
	return uint32(n), nil
}

func uint16Len(n int) (uint16, error) {
	if n < 0 {
		return 0, fmt.Errorf("value %d is outside the uint16 range", n)
	}
	if n > math.MaxUint16 {
		return 0, fmt.Errorf("value %d is outside the uint16 range", n)
	}
	return uint16(n), nil
}

// Encode serializes the request to bytes before SLIP encoding.
// It returns an error when the data length is greater than 65535.
func (r *Request) Encode() ([]byte, error) {
	size, err := uint16Len(len(r.Data))
	if err != nil {
		return nil, err
	}
	packet := make([]byte, 8+len(r.Data))

	packet[0] = DirRequest
	packet[1] = r.Command
	binary.LittleEndian.PutUint16(packet[2:4], size)
	binary.LittleEndian.PutUint32(packet[4:8], r.Checksum)
	copy(packet[8:], r.Data)

	return packet, nil
}

// DecodeResponse parses a response from raw bytes (after SLIP decoding).
// statusBytes selects the trailer layout: protocol.ROMStatusBytes (4) for ROM
// bootloader replies, protocol.StubStatusBytes (2) for stub flasher replies.
func DecodeResponse(data []byte, statusBytes int) (*Response, error) {
	if statusBytes != ROMStatusBytes && statusBytes != StubStatusBytes {
		return nil, fmt.Errorf("invalid status trailer size: %d", statusBytes)
	}
	if len(data) < 10 {
		return nil, fmt.Errorf("response too short: %d bytes", len(data))
	}

	if data[0] != DirResponse {
		return nil, fmt.Errorf("invalid direction byte: 0x%02X", data[0])
	}

	resp := &Response{
		Command: data[1],
	}

	dataSize := binary.LittleEndian.Uint16(data[2:4])
	resp.Value = binary.LittleEndian.Uint32(data[4:8])

	if int(dataSize) > len(data)-8 {
		return nil, fmt.Errorf("data size mismatch: expected %d, have %d", dataSize, len(data)-8)
	}

	if int(dataSize) >= statusBytes {
		// Trailer starts statusBytes before the declared end; status and error
		// sit at its head, any reserved bytes belong to the trailer only.
		trailer := 8 + int(dataSize) - statusBytes
		resp.Data = data[8:trailer]
		resp.Status = data[trailer]
		resp.Error = data[trailer+1]
	} else if dataSize > 0 {
		resp.Data = data[8 : 8+dataSize]
	}

	return resp, nil
}

// IsSuccess returns true if the response indicates success.
func (r *Response) IsSuccess() bool {
	return r.Status == 0 && r.Error == 0
}

// ErrorString returns a human-readable error message.
func (r *Response) ErrorString() string {
	if r.IsSuccess() {
		return ""
	}
	return fmt.Sprintf("status=0x%02X error=0x%02X (%s)", r.Status, r.Error, ErrorMessage(r.Error))
}

// SyncData returns the data payload for a SYNC command.
func SyncData() []byte {
	data := make([]byte, 36)
	data[0] = 0x07
	data[1] = 0x07
	data[2] = 0x12
	data[3] = 0x20
	for i := 4; i < 36; i++ {
		data[i] = 0x55
	}
	return data
}

// FlashEndData creates the data payload for FLASH_END command.
func FlashEndData(reboot bool) []byte {
	data := make([]byte, 4)
	if reboot {
		binary.LittleEndian.PutUint32(data, 0)
	} else {
		binary.LittleEndian.PutUint32(data, 1)
	}
	return data
}

// SpiAttachData creates the data payload for SPI_ATTACH command.
func SpiAttachData() []byte {
	return make([]byte, 8)
}

// SpiSetParamsData creates the data payload for SPI_SET_PARAMS command.
func SpiSetParamsData(totalSize uint32) []byte {
	data := make([]byte, 24)
	binary.LittleEndian.PutUint32(data[0:4], 0)
	binary.LittleEndian.PutUint32(data[4:8], totalSize)
	binary.LittleEndian.PutUint32(data[8:12], 0x10000) // block size (64KB)
	binary.LittleEndian.PutUint32(data[12:16], 0x1000) // sector size (4KB)
	binary.LittleEndian.PutUint32(data[16:20], 0x100)  // page size (256 bytes)
	binary.LittleEndian.PutUint32(data[20:24], 0xFFFF) // status mask
	return data
}

// FlashDeflBeginData creates the data payload for FLASH_DEFL_BEGIN command.
func FlashDeflBeginData(eraseSize, numBlocks, blockSize, offset uint32) []byte {
	data := make([]byte, 16)
	binary.LittleEndian.PutUint32(data[0:4], eraseSize)
	binary.LittleEndian.PutUint32(data[4:8], numBlocks)
	binary.LittleEndian.PutUint32(data[8:12], blockSize)
	binary.LittleEndian.PutUint32(data[12:16], offset)
	return data
}

// FlashDeflDataData creates the data payload for FLASH_DEFL_DATA command.
// It returns an error when the block length is outside the uint32 range.
func FlashDeflDataData(compressedData []byte, seq uint32) ([]byte, error) {
	n, err := Uint32Len(len(compressedData))
	if err != nil {
		return nil, err
	}
	payload := make([]byte, 16+len(compressedData))
	binary.LittleEndian.PutUint32(payload[0:4], n)
	binary.LittleEndian.PutUint32(payload[4:8], seq)
	binary.LittleEndian.PutUint32(payload[8:12], 0)
	binary.LittleEndian.PutUint32(payload[12:16], 0)
	copy(payload[16:], compressedData)
	return payload, nil
}

// FlashDeflEndData creates the data payload for FLASH_DEFL_END command.
func FlashDeflEndData(reboot bool) []byte {
	data := make([]byte, 4)
	if reboot {
		binary.LittleEndian.PutUint32(data, 0)
	} else {
		binary.LittleEndian.PutUint32(data, 1)
	}
	return data
}

// MemBeginData creates the data payload for MEM_BEGIN command.
func MemBeginData(totalSize, numBlocks, blockSize, offset uint32) []byte {
	data := make([]byte, 16)
	binary.LittleEndian.PutUint32(data[0:4], totalSize)
	binary.LittleEndian.PutUint32(data[4:8], numBlocks)
	binary.LittleEndian.PutUint32(data[8:12], blockSize)
	binary.LittleEndian.PutUint32(data[12:16], offset)
	return data
}

// MemDataData creates the data payload for MEM_DATA command.
// It returns an error when the block length is outside the uint32 range.
func MemDataData(blockData []byte, seq uint32) ([]byte, error) {
	n, err := Uint32Len(len(blockData))
	if err != nil {
		return nil, err
	}
	payload := make([]byte, 16+len(blockData))
	binary.LittleEndian.PutUint32(payload[0:4], n)
	binary.LittleEndian.PutUint32(payload[4:8], seq)
	binary.LittleEndian.PutUint32(payload[8:12], 0)
	binary.LittleEndian.PutUint32(payload[12:16], 0)
	copy(payload[16:], blockData)
	return payload, nil
}

// MemEndData creates the data payload for MEM_END command.
func MemEndData(executeFlag, entrypoint uint32) []byte {
	data := make([]byte, 8)
	binary.LittleEndian.PutUint32(data[0:4], executeFlag)
	binary.LittleEndian.PutUint32(data[4:8], entrypoint)
	return data
}

// ReadRegData creates the data payload for READ_REG command.
func ReadRegData(addr uint32) []byte {
	data := make([]byte, 4)
	binary.LittleEndian.PutUint32(data[0:4], addr)
	return data
}

// WriteRegData creates the data payload for WRITE_REG command.
func WriteRegData(addr, value, mask, delayUs uint32) []byte {
	data := make([]byte, 16)
	binary.LittleEndian.PutUint32(data[0:4], addr)
	binary.LittleEndian.PutUint32(data[4:8], value)
	binary.LittleEndian.PutUint32(data[8:12], mask)
	binary.LittleEndian.PutUint32(data[12:16], delayUs)
	return data
}

// ChangeBaudrateData creates the data payload for CHANGE_BAUDRATE command.
func ChangeBaudrateData(newBaud, oldBaud uint32) []byte {
	data := make([]byte, 8)
	binary.LittleEndian.PutUint32(data[0:4], newBaud)
	binary.LittleEndian.PutUint32(data[4:8], oldBaud)
	return data
}

// CalculateDeflBlocks calculates the number of compressed blocks.
// It returns an error when a length is negative, the block size is not positive, or the count is outside the uint32 range.
func CalculateDeflBlocks(compressedLen, blockSize int) (uint32, error) {
	if blockSize <= 0 || compressedLen < 0 || compressedLen > math.MaxInt-blockSize {
		return 0, fmt.Errorf("invalid block inputs %d, %d", compressedLen, blockSize)
	}
	return Uint32Len((compressedLen + blockSize - 1) / blockSize)
}

// CalculateEraseSize calculates the erase size rounded to sector boundary.
// It returns an error when the data length is negative or the size is outside the uint32 range.
func CalculateEraseSize(dataLen int) (uint32, error) {
	if dataLen < 0 || dataLen > math.MaxInt-FlashSectorSize {
		return 0, fmt.Errorf("data length %d is out of range", dataLen)
	}
	size := (dataLen + FlashSectorSize - 1) / FlashSectorSize * FlashSectorSize
	return Uint32Len(size)
}

// SecurityInfo contains chip identity from GET_SECURITY_INFO.
type SecurityInfo struct {
	Flags         uint32
	FlashCryptCnt byte
	ChipID        uint32
}

// ParseSecurityInfo parses the response from GET_SECURITY_INFO command.
// Layout (ESP32-C3/S3): flags u32, flash_crypt_cnt u8, 7 key_purposes u8,
// chip_id u32, api_version u32.
func ParseSecurityInfo(data []byte) (*SecurityInfo, error) {
	if len(data) < 20 {
		return nil, fmt.Errorf("security info too short: %d bytes", len(data))
	}
	return &SecurityInfo{
		Flags:         binary.LittleEndian.Uint32(data[0:4]),
		FlashCryptCnt: data[4],
		ChipID:        binary.LittleEndian.Uint32(data[12:16]),
	}, nil
}
