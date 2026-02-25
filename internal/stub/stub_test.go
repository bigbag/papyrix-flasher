package stub

import (
	"testing"
)

func TestGet_ReturnsValidStub(t *testing.T) {
	s, err := Get()
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if len(s.Text) == 0 {
		t.Error("Get() Text is empty")
	}

	if len(s.Data) == 0 {
		t.Error("Get() Data is empty")
	}

	// ESP32-C3 IRAM starts at 0x40380000
	if s.TextStart < 0x40380000 || s.TextStart > 0x403C0000 {
		t.Errorf("Get() TextStart = 0x%X, want in IRAM range", s.TextStart)
	}

	// ESP32-C3 DRAM starts at 0x3FC80000
	if s.DataStart < 0x3FC80000 || s.DataStart > 0x3FD00000 {
		t.Errorf("Get() DataStart = 0x%X, want in DRAM range", s.DataStart)
	}

	// Entry point should be in IRAM
	if s.Entry < s.TextStart || s.Entry > s.TextStart+uint32(len(s.Text)) {
		t.Errorf("Get() Entry = 0x%X, want within text segment [0x%X, 0x%X]",
			s.Entry, s.TextStart, s.TextStart+uint32(len(s.Text)))
	}
}

func TestGet_IsCached(t *testing.T) {
	s1, err1 := Get()
	s2, err2 := Get()

	if err1 != nil || err2 != nil {
		t.Fatalf("Get() errors: %v, %v", err1, err2)
	}

	if s1 != s2 {
		t.Error("Get() returned different pointers, expected cached result")
	}
}
