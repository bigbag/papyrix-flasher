package stub

import (
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/bigbag/papyrix-flasher/internal/protocol"
)

//go:embed stub_flasher_32c3.json
var stubC3JSON []byte

//go:embed stub_flasher_32s3.json
var stubS3JSON []byte

// Stub contains the decoded stub flasher segments.
type Stub struct {
	Text      []byte
	TextStart uint32
	Data      []byte
	DataStart uint32
	Entry     uint32
}

type stubFile struct {
	Entry     uint32 `json:"entry"`
	Text      string `json:"text"`
	TextStart uint32 `json:"text_start"`
	Data      string `json:"data"`
	DataStart uint32 `json:"data_start"`
}

var (
	getC3 = sync.OnceValues(func() (*Stub, error) { return decode(stubC3JSON) })
	getS3 = sync.OnceValues(func() (*Stub, error) { return decode(stubS3JSON) })
)

// Get returns the decoded stub flasher for chipID.
func Get(chipID uint32) (*Stub, error) {
	switch chipID {
	case protocol.ChipIDESP32C3:
		return getC3()
	case protocol.ChipIDESP32S3:
		return getS3()
	default:
		return nil, fmt.Errorf("unsupported stub chip ID: 0x%02X", chipID)
	}
}

func decode(stubJSON []byte) (*Stub, error) {
	var sf stubFile
	if err := json.Unmarshal(stubJSON, &sf); err != nil {
		return nil, fmt.Errorf("failed to parse stub JSON: %w", err)
	}

	text, err := base64.StdEncoding.DecodeString(sf.Text)
	if err != nil {
		return nil, fmt.Errorf("failed to decode stub text: %w", err)
	}

	data, err := base64.StdEncoding.DecodeString(sf.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode stub data: %w", err)
	}

	return &Stub{
		Text:      text,
		TextStart: sf.TextStart,
		Data:      data,
		DataStart: sf.DataStart,
		Entry:     sf.Entry,
	}, nil
}
