package protocol

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

func TestNewRequest_Checksum_EmptyData(t *testing.T) {
	req := NewRequest(CmdSync, nil)
	// Checksum with no data should be 0xEF (initial value)
	if req.Checksum != 0xEF {
		t.Errorf("NewRequest checksum with empty data = 0x%X, want 0xEF", req.Checksum)
	}
}

func TestNewRequest_Checksum_SingleByte(t *testing.T) {
	// Checksum = 0xEF ^ 0x01 = 0xEE
	req := NewRequest(CmdSync, []byte{0x01})
	if req.Checksum != 0xEE {
		t.Errorf("NewRequest checksum = 0x%X, want 0xEE", req.Checksum)
	}
}

func TestNewRequest_Checksum_MultipleBytes(t *testing.T) {
	// 0xEF ^ 0x01 ^ 0x02 ^ 0x03 = 0xEF ^ 0x00 = 0xEF (0x01^0x02^0x03 = 0)
	req := NewRequest(CmdSync, []byte{0x01, 0x02, 0x03})
	expected := byte(0xEF) ^ 0x01 ^ 0x02 ^ 0x03
	if req.Checksum != uint32(expected) {
		t.Errorf("NewRequest checksum = 0x%X, want 0x%X", req.Checksum, expected)
	}
}

func TestNewRequest_Checksum_SyncData(t *testing.T) {
	syncData := SyncData()
	req := NewRequest(CmdSync, syncData)

	// Calculate expected checksum manually
	var expected byte = 0xEF
	for _, b := range syncData {
		expected ^= b
	}

	if req.Checksum != uint32(expected) {
		t.Errorf("NewRequest checksum for SyncData = 0x%X, want 0x%X", req.Checksum, expected)
	}
}

func TestNewRequest_Fields(t *testing.T) {
	data := []byte{0x01, 0x02, 0x03}
	req := NewRequest(CmdFlashEnd, data)

	if req.Command != CmdFlashEnd {
		t.Errorf("NewRequest Command = 0x%02X, want 0x%02X", req.Command, CmdFlashEnd)
	}
	if !bytes.Equal(req.Data, data) {
		t.Errorf("NewRequest Data = %v, want %v", req.Data, data)
	}
}

func TestRequest_Encode_Format(t *testing.T) {
	data := []byte{0xAA, 0xBB}
	req := NewRequest(CmdSync, data)
	encoded := req.Encode()

	// Format: direction(1) + cmd(1) + len(2) + checksum(4) + data
	expectedLen := 8 + len(data)
	if len(encoded) != expectedLen {
		t.Fatalf("Encode() length = %d, want %d", len(encoded), expectedLen)
	}

	// Check direction byte
	if encoded[0] != DirRequest {
		t.Errorf("Encode()[0] direction = 0x%02X, want 0x%02X", encoded[0], DirRequest)
	}

	// Check command byte
	if encoded[1] != CmdSync {
		t.Errorf("Encode()[1] command = 0x%02X, want 0x%02X", encoded[1], CmdSync)
	}

	// Check length (little-endian)
	dataLen := binary.LittleEndian.Uint16(encoded[2:4])
	if dataLen != uint16(len(data)) {
		t.Errorf("Encode() data length = %d, want %d", dataLen, len(data))
	}

	// Check checksum (little-endian)
	checksum := binary.LittleEndian.Uint32(encoded[4:8])
	if checksum != req.Checksum {
		t.Errorf("Encode() checksum = 0x%X, want 0x%X", checksum, req.Checksum)
	}

	// Check data
	if !bytes.Equal(encoded[8:], data) {
		t.Errorf("Encode() data = %v, want %v", encoded[8:], data)
	}
}

func TestRequest_Encode_EmptyData(t *testing.T) {
	req := NewRequest(CmdSync, nil)
	encoded := req.Encode()

	if len(encoded) != 8 {
		t.Fatalf("Encode() length = %d, want 8", len(encoded))
	}

	dataLen := binary.LittleEndian.Uint16(encoded[2:4])
	if dataLen != 0 {
		t.Errorf("Encode() data length = %d, want 0", dataLen)
	}
}

func TestRequest_Encode_LargeData(t *testing.T) {
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i % 256)
	}

	req := NewRequest(CmdFlashDeflData, data)
	encoded := req.Encode()

	if len(encoded) != 8+len(data) {
		t.Fatalf("Encode() length = %d, want %d", len(encoded), 8+len(data))
	}

	dataLen := binary.LittleEndian.Uint16(encoded[2:4])
	if dataLen != uint16(len(data)) {
		t.Errorf("Encode() data length = %d, want %d", dataLen, len(data))
	}
}

func TestDecodeResponse_Valid(t *testing.T) {
	// Build a valid response: direction(1) + cmd(1) + size(2) + value(4) + data(2) = status + error
	resp := make([]byte, 10)
	resp[0] = DirResponse
	resp[1] = CmdSync
	binary.LittleEndian.PutUint16(resp[2:4], 2) // size = 2 (status + error)
	binary.LittleEndian.PutUint32(resp[4:8], 0x12345678)
	resp[8] = 0x00 // status
	resp[9] = 0x00 // error

	decoded, err := DecodeResponse(resp)
	if err != nil {
		t.Fatalf("DecodeResponse() error = %v", err)
	}

	if decoded.Command != CmdSync {
		t.Errorf("DecodeResponse Command = 0x%02X, want 0x%02X", decoded.Command, CmdSync)
	}
	if decoded.Value != 0x12345678 {
		t.Errorf("DecodeResponse Value = 0x%X, want 0x12345678", decoded.Value)
	}
	if decoded.Status != 0 {
		t.Errorf("DecodeResponse Status = 0x%02X, want 0x00", decoded.Status)
	}
	if decoded.Error != 0 {
		t.Errorf("DecodeResponse Error = 0x%02X, want 0x00", decoded.Error)
	}
}

func TestDecodeResponse_WithData(t *testing.T) {
	// Response with additional data beyond status/error
	extra := []byte{0xAA, 0xBB, 0xCC}
	dataSize := uint16(len(extra) + 2) // extra + status + error

	resp := make([]byte, 8+int(dataSize))
	resp[0] = DirResponse
	resp[1] = CmdGetSecurityInfo
	binary.LittleEndian.PutUint16(resp[2:4], dataSize)
	binary.LittleEndian.PutUint32(resp[4:8], 0)
	copy(resp[8:], extra)
	resp[8+len(extra)] = 0x00   // status
	resp[8+len(extra)+1] = 0x00 // error

	decoded, err := DecodeResponse(resp)
	if err != nil {
		t.Fatalf("DecodeResponse() error = %v", err)
	}

	if !bytes.Equal(decoded.Data, extra) {
		t.Errorf("DecodeResponse Data = %v, want %v", decoded.Data, extra)
	}
}

func TestDecodeResponse_TooShort(t *testing.T) {
	shortResponses := [][]byte{
		nil,
		{},
		{DirResponse},
		make([]byte, 9),
	}

	for _, resp := range shortResponses {
		_, err := DecodeResponse(resp)
		if err == nil {
			t.Errorf("DecodeResponse(%v) expected error, got nil", resp)
		}
	}
}

func TestDecodeResponse_InvalidDirection(t *testing.T) {
	resp := make([]byte, 10)
	resp[0] = DirRequest // Wrong direction
	resp[1] = CmdSync
	binary.LittleEndian.PutUint16(resp[2:4], 2)

	_, err := DecodeResponse(resp)
	if err == nil {
		t.Error("DecodeResponse with wrong direction expected error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid direction") {
		t.Errorf("DecodeResponse error = %v, want error containing 'invalid direction'", err)
	}
}

func TestDecodeResponse_DataSizeMismatch(t *testing.T) {
	resp := make([]byte, 10)
	resp[0] = DirResponse
	resp[1] = CmdSync
	binary.LittleEndian.PutUint16(resp[2:4], 100) // Claims 100 bytes but only has 2

	_, err := DecodeResponse(resp)
	if err == nil {
		t.Error("DecodeResponse with size mismatch expected error, got nil")
	}
	if !strings.Contains(err.Error(), "size mismatch") {
		t.Errorf("DecodeResponse error = %v, want error containing 'size mismatch'", err)
	}
}

func TestDecodeResponse_ZeroDataSize(t *testing.T) {
	resp := make([]byte, 10)
	resp[0] = DirResponse
	resp[1] = CmdSync
	binary.LittleEndian.PutUint16(resp[2:4], 0) // No data
	binary.LittleEndian.PutUint32(resp[4:8], 0x12345678)

	decoded, err := DecodeResponse(resp)
	if err != nil {
		t.Fatalf("DecodeResponse() error = %v", err)
	}

	if len(decoded.Data) != 0 {
		t.Errorf("DecodeResponse Data = %v, want empty", decoded.Data)
	}
}

func TestResponse_IsSuccess(t *testing.T) {
	tests := []struct {
		status   byte
		errCode  byte
		expected bool
	}{
		{0, 0, true},
		{1, 0, false},
		{0, 1, false},
		{1, 1, false},
		{0xFF, 0, false},
		{0, 0xFF, false},
	}

	for _, tc := range tests {
		resp := &Response{Status: tc.status, Error: tc.errCode}
		result := resp.IsSuccess()
		if result != tc.expected {
			t.Errorf("IsSuccess(status=0x%02X, error=0x%02X) = %v, want %v",
				tc.status, tc.errCode, result, tc.expected)
		}
	}
}

func TestResponse_ErrorString_Success(t *testing.T) {
	resp := &Response{Status: 0, Error: 0}
	result := resp.ErrorString()
	if result != "" {
		t.Errorf("ErrorString() for success = %q, want empty", result)
	}
}

func TestResponse_ErrorString_Failure(t *testing.T) {
	resp := &Response{Status: 1, Error: ErrInvalidCRC}
	result := resp.ErrorString()

	if !strings.Contains(result, "0x01") {
		t.Errorf("ErrorString() = %q, should contain status '0x01'", result)
	}
	if !strings.Contains(result, "0x07") {
		t.Errorf("ErrorString() = %q, should contain error code '0x07'", result)
	}
	if !strings.Contains(result, "invalid CRC") {
		t.Errorf("ErrorString() = %q, should contain 'invalid CRC'", result)
	}
}

func TestResponse_ErrorString_AllErrorCodes(t *testing.T) {
	errorCodes := []byte{
		ErrInvalidMessage,
		ErrFailedToAct,
		ErrInvalidCRC,
		ErrFlashWriteErr,
		ErrFlashReadErr,
		ErrFlashReadLenErr,
		ErrDeflateError,
	}

	for _, code := range errorCodes {
		resp := &Response{Status: 1, Error: code}
		result := resp.ErrorString()
		if result == "" {
			t.Errorf("ErrorString() for error 0x%02X = empty, want non-empty", code)
		}
		// Should contain the error message from ErrorMessage()
		expectedMsg := ErrorMessage(code)
		if !strings.Contains(result, expectedMsg) {
			t.Errorf("ErrorString() = %q, should contain %q", result, expectedMsg)
		}
	}
}

func TestResponse_ErrorString_UnknownError(t *testing.T) {
	resp := &Response{Status: 1, Error: 0x99}
	result := resp.ErrorString()

	if !strings.Contains(result, "unknown error") {
		t.Errorf("ErrorString() = %q, should contain 'unknown error'", result)
	}
}

func TestDirectionConstants(t *testing.T) {
	if DirRequest != 0x00 {
		t.Errorf("DirRequest = 0x%02X, want 0x00", DirRequest)
	}
	if DirResponse != 0x01 {
		t.Errorf("DirResponse = 0x%02X, want 0x01", DirResponse)
	}
}
