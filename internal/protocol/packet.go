package protocol

import (
	"encoding/binary"
	"fmt"
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

// NewRequest creates a new request with calculated checksum.
func NewRequest(cmd byte, data []byte) *Request {
	r := &Request{
		Command: cmd,
		Data:    data,
	}
	r.Checksum = r.calculateChecksum()
	return r
}

// calculateChecksum computes the checksum for the request data.
func (r *Request) calculateChecksum() uint32 {
	var checksum byte = 0xEF
	for _, b := range r.Data {
		checksum ^= b
	}
	return uint32(checksum)
}

// Encode serializes the request to bytes (before SLIP encoding).
func (r *Request) Encode() []byte {
	size := uint16(len(r.Data))
	packet := make([]byte, 8+len(r.Data))

	packet[0] = DirRequest
	packet[1] = r.Command
	binary.LittleEndian.PutUint16(packet[2:4], size)
	binary.LittleEndian.PutUint32(packet[4:8], r.Checksum)
	copy(packet[8:], r.Data)

	return packet
}

// DecodeResponse parses a response from raw bytes (after SLIP decoding).
func DecodeResponse(data []byte) (*Response, error) {
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

	if dataSize >= 2 {
		resp.Data = data[8 : 8+dataSize-2]
		resp.Status = data[8+dataSize-2]
		resp.Error = data[8+dataSize-1]
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
func FlashDeflDataData(compressedData []byte, seq uint32) []byte {
	payload := make([]byte, 16+len(compressedData))
	binary.LittleEndian.PutUint32(payload[0:4], uint32(len(compressedData)))
	binary.LittleEndian.PutUint32(payload[4:8], seq)
	binary.LittleEndian.PutUint32(payload[8:12], 0)
	binary.LittleEndian.PutUint32(payload[12:16], 0)
	copy(payload[16:], compressedData)
	return payload
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

// CalculateDeflBlocks calculates the number of compressed blocks.
func CalculateDeflBlocks(compressedLen, blockSize int) uint32 {
	return uint32((compressedLen + blockSize - 1) / blockSize)
}

// CalculateEraseSize calculates the erase size rounded to sector boundary.
func CalculateEraseSize(dataLen int) uint32 {
	return uint32((dataLen + FlashSectorSize - 1) / FlashSectorSize * FlashSectorSize)
}

// ESP32C3Info contains chip information.
type ESP32C3Info struct {
	ChipID uint32
}

// ParseSecurityInfo parses the response from GET_SECURITY_INFO command.
func ParseSecurityInfo(data []byte) (*ESP32C3Info, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("security info too short: %d bytes", len(data))
	}
	return &ESP32C3Info{
		ChipID: binary.LittleEndian.Uint32(data[0:4]),
	}, nil
}
