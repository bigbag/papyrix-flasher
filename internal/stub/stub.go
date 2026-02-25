package stub

import (
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"
)

//go:embed stub_flasher_32c3.json
var stubJSON []byte

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
	cachedStub  *Stub
	cachedErr   error
	decodeOnce  sync.Once
)

// Get returns the decoded ESP32-C3 stub flasher.
func Get() (*Stub, error) {
	decodeOnce.Do(func() {
		var sf stubFile
		if err := json.Unmarshal(stubJSON, &sf); err != nil {
			cachedErr = fmt.Errorf("failed to parse stub JSON: %w", err)
			return
		}

		text, err := base64.StdEncoding.DecodeString(sf.Text)
		if err != nil {
			cachedErr = fmt.Errorf("failed to decode stub text: %w", err)
			return
		}

		var data []byte
		if sf.Data != "" {
			data, err = base64.StdEncoding.DecodeString(sf.Data)
			if err != nil {
				cachedErr = fmt.Errorf("failed to decode stub data: %w", err)
				return
			}
		}

		cachedStub = &Stub{
			Text:      text,
			TextStart: sf.TextStart,
			Data:      data,
			DataStart: sf.DataStart,
			Entry:     sf.Entry,
		}
	})
	return cachedStub, cachedErr
}
