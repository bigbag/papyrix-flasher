package flasher

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
	"time"

	"github.com/bigbag/papyrix-flasher/internal/protocol"
	"github.com/bigbag/papyrix-flasher/internal/slip"
)

func responseFrame(command byte, payload []byte, statusBytes int, status, errorCode byte) []byte {
	raw := make([]byte, 8+len(payload)+statusBytes)
	raw[0] = protocol.DirResponse
	raw[1] = command
	binary.LittleEndian.PutUint16(raw[2:4], uint16(len(payload)+statusBytes))
	copy(raw[8:], payload)
	raw[8+len(payload)] = status
	raw[8+len(payload)+1] = errorCode
	return slip.Encode(raw)
}

func TestReadResponse_SkipsUnexpectedCommand(t *testing.T) {
	payload := make([]byte, 20)
	binary.LittleEndian.PutUint32(payload[12:16], protocol.ChipIDESP32S3)
	buf := append(
		responseFrame(protocol.CmdSync, nil, protocol.ROMStatusBytes, 0, 0),
		responseFrame(protocol.CmdGetSecurityInfo, payload, protocol.ROMStatusBytes, 0, 0)...,
	)
	f := &Flasher{buf: buf}

	resp, err := f.readResponse(protocol.CmdGetSecurityInfo, time.Millisecond)
	if err != nil {
		t.Fatalf("readResponse() error = %v", err)
	}
	if resp.Command != protocol.CmdGetSecurityInfo || !bytes.Equal(resp.Data, payload) {
		t.Errorf("readResponse() returned command 0x%02X data %v", resp.Command, resp.Data)
	}
}

func TestReadResponse_UsesStubTrailer(t *testing.T) {
	payload := []byte{0xAA, 0xBB}
	f := &Flasher{
		stubRunning: true,
		buf:         responseFrame(protocol.CmdReadReg, payload, protocol.StubStatusBytes, 0, 0),
	}

	resp, err := f.readResponse(protocol.CmdReadReg, time.Millisecond)
	if err != nil {
		t.Fatalf("readResponse() error = %v", err)
	}
	if !bytes.Equal(resp.Data, payload) || !resp.IsSuccess() {
		t.Errorf("readResponse() = data %v, status %d, error %d", resp.Data, resp.Status, resp.Error)
	}
}

func TestValidateTarget(t *testing.T) {
	tests := []struct {
		name     string
		info     protocol.SecurityInfo
		expected uint32
		wantErr  string
	}{
		{"valid C3", protocol.SecurityInfo{ChipID: protocol.ChipIDESP32C3}, protocol.ChipIDESP32C3, ""},
		{"valid S3", protocol.SecurityInfo{ChipID: protocol.ChipIDESP32S3, FlashCryptCnt: 3}, protocol.ChipIDESP32S3, ""},
		{"unsupported", protocol.SecurityInfo{ChipID: 2}, protocol.ChipIDESP32C3, "unsupported"},
		{"target mismatch", protocol.SecurityInfo{ChipID: protocol.ChipIDESP32C3}, protocol.ChipIDESP32S3, "targets"},
		{"secure boot", protocol.SecurityInfo{ChipID: protocol.ChipIDESP32C3, Flags: 1}, protocol.ChipIDESP32C3, "secure"},
		{"secure download", protocol.SecurityInfo{ChipID: protocol.ChipIDESP32C3, Flags: 4}, protocol.ChipIDESP32C3, "secure"},
		{"flash encryption", protocol.SecurityInfo{ChipID: protocol.ChipIDESP32C3, FlashCryptCnt: 1}, protocol.ChipIDESP32C3, "encryption"},
	}

	for _, tc := range tests {
		err := validateTarget(&tc.info, tc.expected)
		if tc.wantErr == "" && err != nil {
			t.Errorf("%s: validateTarget() error = %v", tc.name, err)
		}
		if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
			t.Errorf("%s: validateTarget() error = %v, want containing %q", tc.name, err, tc.wantErr)
		}
	}
}

func TestWatchdogConfig(t *testing.T) {
	tests := []struct {
		chipID uint32
		want   watchdogConfig
	}{
		{protocol.ChipIDESP32C3, watchdogConfig{0x3FCDF07C, 3, 0x60008090, 0x600080A8, 0x600080AC, 0x600080B0}},
		{protocol.ChipIDESP32S3, watchdogConfig{0x3FCEF14C, 4, 0x60008098, 0x600080B0, 0x600080B4, 0x600080B8}},
	}

	for _, tc := range tests {
		got, err := configForChip(tc.chipID)
		if err != nil {
			t.Fatalf("configForChip(0x%X) error = %v", tc.chipID, err)
		}
		if got != tc.want {
			t.Errorf("configForChip(0x%X) = %#v, want %#v", tc.chipID, got, tc.want)
		}
	}
	if _, err := configForChip(2); err == nil {
		t.Fatal("configForChip() accepted unsupported chip")
	}
}
