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
// Checksum is XOR of all data bytes.
func (r *Request) calculateChecksum() uint32 {
	var checksum byte = 0xEF
	for _, b := range r.Data {
		checksum ^= b
	}
	return uint32(checksum)
}

// Encode serializes the request to bytes (before SLIP encoding).
func (r *Request) Encode() []byte {
	// Packet format:
	// 0: direction (0x00 = request)
	// 1: command
	// 2-3: data size (little-endian)
	// 4-7: checksum (little-endian, only for data commands)
	// 8+: data

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
	// Minimum response is 8 bytes header + 2 bytes status
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

	// Response data follows header
	if int(dataSize) > len(data)-8 {
		return nil, fmt.Errorf("data size mismatch: expected %d, have %d", dataSize, len(data)-8)
	}

	if dataSize >= 2 {
		// Last two bytes are status and error
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
	// SYNC payload: 0x07 0x07 0x12 0x20 followed by 32 bytes of 0x55
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

// FlashBeginData creates the data payload for FLASH_BEGIN command.
func FlashBeginData(size, numBlocks, blockSize, offset uint32) []byte {
	data := make([]byte, 16)
	binary.LittleEndian.PutUint32(data[0:4], size)
	binary.LittleEndian.PutUint32(data[4:8], numBlocks)
	binary.LittleEndian.PutUint32(data[8:12], blockSize)
	binary.LittleEndian.PutUint32(data[12:16], offset)
	return data
}

// FlashDataData creates the data payload for FLASH_DATA command.
func FlashDataData(data []byte, seq uint32) []byte {
	// Pad data to block size if needed
	blockSize := FlashBlockSize
	if len(data) < blockSize {
		padded := make([]byte, blockSize)
		copy(padded, data)
		for i := len(data); i < blockSize; i++ {
			padded[i] = 0xFF
		}
		data = padded
	}

	// Header: size (4) + seq (4) + reserved (8)
	payload := make([]byte, 16+len(data))
	binary.LittleEndian.PutUint32(payload[0:4], uint32(len(data)))
	binary.LittleEndian.PutUint32(payload[4:8], seq)
	binary.LittleEndian.PutUint32(payload[8:12], 0)
	binary.LittleEndian.PutUint32(payload[12:16], 0)
	copy(payload[16:], data)

	return payload
}

// FlashEndData creates the data payload for FLASH_END command.
func FlashEndData(reboot bool) []byte {
	data := make([]byte, 4)
	if reboot {
		binary.LittleEndian.PutUint32(data, 0) // 0 = reboot
	} else {
		binary.LittleEndian.PutUint32(data, 1) // 1 = stay in bootloader
	}
	return data
}

// FlashMD5Data creates the data payload for SPI_FLASH_MD5 command.
func FlashMD5Data(address, size uint32) []byte {
	data := make([]byte, 16)
	binary.LittleEndian.PutUint32(data[0:4], address)
	binary.LittleEndian.PutUint32(data[4:8], size)
	binary.LittleEndian.PutUint32(data[8:12], 0)
	binary.LittleEndian.PutUint32(data[12:16], 0)
	return data
}

// SpiAttachData creates the data payload for SPI_ATTACH command.
func SpiAttachData() []byte {
	// For ESP32-C3: all zeros means use default SPI configuration
	return make([]byte, 8)
}
