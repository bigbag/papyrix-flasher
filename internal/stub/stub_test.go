package stub

import (
	"testing"

	"github.com/bigbag/papyrix-flasher/internal/protocol"
)

func TestGet_ReturnsValidC3Stub(t *testing.T) {
	s, err := Get(protocol.ChipIDESP32C3)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	checkStub(t, s, 0x40380000, 0x403C0000, 0x3FC80000, 0x3FD00000)
}

func TestGet_ReturnsValidS3Stub(t *testing.T) {
	s, err := Get(protocol.ChipIDESP32S3)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	checkStub(t, s, 0x40370000, 0x403E0000, 0x3FC88000, 0x3FD00000)
}

func TestGet_RejectsUnsupportedChip(t *testing.T) {
	if _, err := Get(0x02); err == nil {
		t.Fatal("Get() accepted unsupported chip")
	}
}

func checkStub(t *testing.T, s *Stub, textMin, textMax, dataMin, dataMax uint32) {
	t.Helper()
	if len(s.Text) == 0 {
		t.Fatal("stub text is empty")
	}
	if s.TextStart < textMin || s.TextStart >= textMax {
		t.Errorf("TextStart = 0x%X, want [0x%X, 0x%X)", s.TextStart, textMin, textMax)
	}
	if s.Entry < s.TextStart || s.Entry >= s.TextStart+uint32(len(s.Text)) {
		t.Errorf("Entry = 0x%X, want within text [0x%X, 0x%X)", s.Entry, s.TextStart, s.TextStart+uint32(len(s.Text)))
	}
	if len(s.Data) > 0 && (s.DataStart < dataMin || s.DataStart >= dataMax) {
		t.Errorf("DataStart = 0x%X, want [0x%X, 0x%X)", s.DataStart, dataMin, dataMax)
	}
}
