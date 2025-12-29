package slip

const (
	End     = 0xC0
	Esc     = 0xDB
	EscEnd  = 0xDC
	EscEsc  = 0xDD
)

// Encode wraps data in SLIP framing.
// Adds END byte at start and end, escapes special bytes.
func Encode(data []byte) []byte {
	// Pre-allocate with some extra space for escapes
	result := make([]byte, 0, len(data)+10)
	result = append(result, End)

	for _, b := range data {
		switch b {
		case End:
			result = append(result, Esc, EscEnd)
		case Esc:
			result = append(result, Esc, EscEsc)
		default:
			result = append(result, b)
		}
	}

	result = append(result, End)
	return result
}

// Decode extracts data from a SLIP frame.
// Removes END bytes and unescapes special bytes.
func Decode(frame []byte) []byte {
	if len(frame) < 2 {
		return nil
	}

	// Strip leading/trailing END bytes
	start := 0
	end := len(frame)

	for start < end && frame[start] == End {
		start++
	}
	for end > start && frame[end-1] == End {
		end--
	}

	if start >= end {
		return nil
	}

	data := frame[start:end]
	result := make([]byte, 0, len(data))

	i := 0
	for i < len(data) {
		if data[i] == Esc && i+1 < len(data) {
			switch data[i+1] {
			case EscEnd:
				result = append(result, End)
			case EscEsc:
				result = append(result, Esc)
			default:
				result = append(result, data[i+1])
			}
			i += 2
		} else {
			result = append(result, data[i])
			i++
		}
	}

	return result
}

// ReadFrame reads a complete SLIP frame from a byte stream.
// Returns the frame (including END delimiters) and remaining bytes.
func ReadFrame(data []byte) (frame []byte, remaining []byte) {
	// Find start of frame (skip leading END bytes or find first END)
	start := -1
	for i, b := range data {
		if b == End {
			start = i
			break
		}
	}

	if start == -1 {
		return nil, data
	}

	// Find end of frame (next END after some data)
	inFrame := false
	for i := start; i < len(data); i++ {
		if data[i] == End {
			if inFrame {
				// Found the closing END
				return data[start : i+1], data[i+1:]
			}
		} else {
			inFrame = true
		}
	}

	// Frame not complete yet
	return nil, data
}
