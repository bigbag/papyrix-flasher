package embedded

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/bigbag/papyrix-flasher/internal/protocol"
)

func TestBootloader_SelectsMatchingChip(t *testing.T) {
	for _, chipID := range []uint32{protocol.ChipIDESP32C3, protocol.ChipIDESP32S3} {
		data, err := Bootloader(chipID)
		if err != nil {
			t.Fatalf("Bootloader(0x%X) error = %v", chipID, err)
		}
		if len(data) < 14 || data[0] != 0xE9 {
			t.Fatalf("Bootloader(0x%X) is not an ESP image", chipID)
		}
		if got := uint32(binary.LittleEndian.Uint16(data[12:14])); got != chipID {
			t.Errorf("Bootloader(0x%X) image chip = 0x%X", chipID, got)
		}
	}
}

func TestBootloader_RejectsUnsupportedChip(t *testing.T) {
	if _, err := Bootloader(0x02); err == nil {
		t.Fatal("Bootloader() accepted unsupported chip")
	}
}

func TestPartitions_MatchPapyrixLayout(t *testing.T) {
	want := map[string][2]uint32{
		"nvs":      {0x9000, 0x5000},
		"otadata":  {0xE000, 0x2000},
		"app0":     {0x10000, 0x640000},
		"app1":     {0x650000, 0x640000},
		"spiffs":   {0xC90000, 0x360000},
		"coredump": {0xFF0000, 0x10000},
	}
	got := make(map[string][2]uint32)
	data := Partitions()

	for offset := 0; offset+32 <= len(data); offset += 32 {
		entry := data[offset : offset+32]
		magic := binary.LittleEndian.Uint16(entry[0:2])
		if magic == 0xFFFF || magic == 0xEBEB {
			break
		}
		if magic != 0x50AA {
			t.Fatalf("partition entry at %d has magic 0x%04X", offset, magic)
		}
		name := strings.TrimRight(string(entry[12:28]), "\x00")
		got[name] = [2]uint32{
			binary.LittleEndian.Uint32(entry[4:8]),
			binary.LittleEndian.Uint32(entry[8:12]),
		}
	}

	for name, values := range want {
		if got[name] != values {
			t.Errorf("partition %q = %#v, want %#v", name, got[name], values)
		}
	}
}
