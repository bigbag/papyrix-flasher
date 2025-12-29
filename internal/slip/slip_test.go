package slip

import (
	"bytes"
	"testing"
)

func TestEncode_EmptyData(t *testing.T) {
	result := Encode(nil)
	expected := []byte{End, End}
	if !bytes.Equal(result, expected) {
		t.Errorf("Encode(nil) = %v, want %v", result, expected)
	}

	result = Encode([]byte{})
	if !bytes.Equal(result, expected) {
		t.Errorf("Encode([]) = %v, want %v", result, expected)
	}
}

func TestEncode_NoSpecialBytes(t *testing.T) {
	input := []byte{0x01, 0x02, 0x03, 0x04}
	result := Encode(input)
	expected := []byte{End, 0x01, 0x02, 0x03, 0x04, End}
	if !bytes.Equal(result, expected) {
		t.Errorf("Encode(%v) = %v, want %v", input, result, expected)
	}
}

func TestEncode_EscapeEndByte(t *testing.T) {
	input := []byte{0x01, End, 0x03}
	result := Encode(input)
	expected := []byte{End, 0x01, Esc, EscEnd, 0x03, End}
	if !bytes.Equal(result, expected) {
		t.Errorf("Encode(%v) = %v, want %v", input, result, expected)
	}
}

func TestEncode_EscapeEscByte(t *testing.T) {
	input := []byte{0x01, Esc, 0x03}
	result := Encode(input)
	expected := []byte{End, 0x01, Esc, EscEsc, 0x03, End}
	if !bytes.Equal(result, expected) {
		t.Errorf("Encode(%v) = %v, want %v", input, result, expected)
	}
}

func TestEncode_MultipleSpecialBytes(t *testing.T) {
	input := []byte{End, Esc, End, Esc}
	result := Encode(input)
	expected := []byte{End, Esc, EscEnd, Esc, EscEsc, Esc, EscEnd, Esc, EscEsc, End}
	if !bytes.Equal(result, expected) {
		t.Errorf("Encode(%v) = %v, want %v", input, result, expected)
	}
}

func TestEncode_AllSpecialBytes(t *testing.T) {
	// Test data that's all special bytes
	input := []byte{End, End, Esc, Esc}
	result := Encode(input)
	expected := []byte{End, Esc, EscEnd, Esc, EscEnd, Esc, EscEsc, Esc, EscEsc, End}
	if !bytes.Equal(result, expected) {
		t.Errorf("Encode(%v) = %v, want %v", input, result, expected)
	}
}

func TestDecode_ValidFrame(t *testing.T) {
	frame := []byte{End, 0x01, 0x02, 0x03, End}
	result := Decode(frame)
	expected := []byte{0x01, 0x02, 0x03}
	if !bytes.Equal(result, expected) {
		t.Errorf("Decode(%v) = %v, want %v", frame, result, expected)
	}
}

func TestDecode_UnescapeEndByte(t *testing.T) {
	frame := []byte{End, 0x01, Esc, EscEnd, 0x03, End}
	result := Decode(frame)
	expected := []byte{0x01, End, 0x03}
	if !bytes.Equal(result, expected) {
		t.Errorf("Decode(%v) = %v, want %v", frame, result, expected)
	}
}

func TestDecode_UnescapeEscByte(t *testing.T) {
	frame := []byte{End, 0x01, Esc, EscEsc, 0x03, End}
	result := Decode(frame)
	expected := []byte{0x01, Esc, 0x03}
	if !bytes.Equal(result, expected) {
		t.Errorf("Decode(%v) = %v, want %v", frame, result, expected)
	}
}

func TestDecode_EmptyFrame(t *testing.T) {
	frame := []byte{End, End}
	result := Decode(frame)
	if result != nil {
		t.Errorf("Decode(%v) = %v, want nil", frame, result)
	}
}

func TestDecode_TooShort(t *testing.T) {
	result := Decode([]byte{End})
	if result != nil {
		t.Errorf("Decode([0xC0]) = %v, want nil", result)
	}

	result = Decode(nil)
	if result != nil {
		t.Errorf("Decode(nil) = %v, want nil", result)
	}
}

func TestDecode_MultipleLeadingEndBytes(t *testing.T) {
	frame := []byte{End, End, End, 0x01, 0x02, End}
	result := Decode(frame)
	expected := []byte{0x01, 0x02}
	if !bytes.Equal(result, expected) {
		t.Errorf("Decode(%v) = %v, want %v", frame, result, expected)
	}
}

func TestDecode_MultipleTrailingEndBytes(t *testing.T) {
	frame := []byte{End, 0x01, 0x02, End, End, End}
	result := Decode(frame)
	expected := []byte{0x01, 0x02}
	if !bytes.Equal(result, expected) {
		t.Errorf("Decode(%v) = %v, want %v", frame, result, expected)
	}
}

func TestDecode_UnknownEscapeSequence(t *testing.T) {
	// Unknown escape sequence should pass through the second byte
	frame := []byte{End, 0x01, Esc, 0xFF, 0x03, End}
	result := Decode(frame)
	expected := []byte{0x01, 0xFF, 0x03}
	if !bytes.Equal(result, expected) {
		t.Errorf("Decode(%v) = %v, want %v", frame, result, expected)
	}
}

func TestEncodeDecode_RoundTrip(t *testing.T) {
	testCases := [][]byte{
		{},
		{0x00},
		{0x01, 0x02, 0x03},
		{End},
		{Esc},
		{End, Esc},
		{0x00, End, 0x00, Esc, 0x00},
		{0xFF, 0xFE, 0xFD},
		// Large data
		make([]byte, 256),
	}

	for i, tc := range testCases {
		encoded := Encode(tc)
		decoded := Decode(encoded)
		if !bytes.Equal(decoded, tc) {
			t.Errorf("Case %d: RoundTrip(%v) = %v, want %v", i, tc, decoded, tc)
		}
	}
}

func TestReadFrame_SingleFrame(t *testing.T) {
	data := []byte{End, 0x01, 0x02, 0x03, End}
	frame, remaining := ReadFrame(data)
	if !bytes.Equal(frame, data) {
		t.Errorf("ReadFrame(%v) frame = %v, want %v", data, frame, data)
	}
	if len(remaining) != 0 {
		t.Errorf("ReadFrame(%v) remaining = %v, want []", data, remaining)
	}
}

func TestReadFrame_MultipleFrames(t *testing.T) {
	frame1 := []byte{End, 0x01, 0x02, End}
	frame2 := []byte{End, 0x03, 0x04, End}
	data := append(append([]byte{}, frame1...), frame2...)

	frame, remaining := ReadFrame(data)
	if !bytes.Equal(frame, frame1) {
		t.Errorf("ReadFrame first frame = %v, want %v", frame, frame1)
	}
	if !bytes.Equal(remaining, frame2) {
		t.Errorf("ReadFrame remaining = %v, want %v", remaining, frame2)
	}
}

func TestReadFrame_IncompleteFrame(t *testing.T) {
	data := []byte{End, 0x01, 0x02}
	frame, remaining := ReadFrame(data)
	if frame != nil {
		t.Errorf("ReadFrame incomplete = %v, want nil", frame)
	}
	if !bytes.Equal(remaining, data) {
		t.Errorf("ReadFrame remaining = %v, want %v", remaining, data)
	}
}

func TestReadFrame_NoFrame(t *testing.T) {
	data := []byte{0x01, 0x02, 0x03}
	frame, remaining := ReadFrame(data)
	if frame != nil {
		t.Errorf("ReadFrame no frame = %v, want nil", frame)
	}
	if !bytes.Equal(remaining, data) {
		t.Errorf("ReadFrame remaining = %v, want %v", remaining, data)
	}
}

func TestReadFrame_EmptyInput(t *testing.T) {
	frame, remaining := ReadFrame(nil)
	if frame != nil {
		t.Errorf("ReadFrame(nil) frame = %v, want nil", frame)
	}
	if remaining != nil {
		t.Errorf("ReadFrame(nil) remaining = %v, want nil", remaining)
	}

	frame, remaining = ReadFrame([]byte{})
	if frame != nil {
		t.Errorf("ReadFrame([]) frame = %v, want nil", frame)
	}
	if len(remaining) != 0 {
		t.Errorf("ReadFrame([]) remaining = %v, want []", remaining)
	}
}

func TestReadFrame_OnlyEndBytes(t *testing.T) {
	// Multiple END bytes with no data between them
	data := []byte{End, End, End}
	frame, _ := ReadFrame(data)
	if frame != nil {
		t.Errorf("ReadFrame only ENDs = %v, want nil", frame)
	}
}

func TestReadFrame_LeadingGarbage(t *testing.T) {
	// Data before the first END should be skipped
	data := []byte{0x01, 0x02, End, 0x03, 0x04, End}
	frame, remaining := ReadFrame(data)
	expected := []byte{End, 0x03, 0x04, End}
	if !bytes.Equal(frame, expected) {
		t.Errorf("ReadFrame with garbage = %v, want %v", frame, expected)
	}
	if len(remaining) != 0 {
		t.Errorf("ReadFrame remaining = %v, want []", remaining)
	}
}

func TestReadFrame_FrameWithEscapes(t *testing.T) {
	// Frame containing escaped bytes should be returned as-is
	data := []byte{End, 0x01, Esc, EscEnd, 0x02, End}
	frame, remaining := ReadFrame(data)
	if !bytes.Equal(frame, data) {
		t.Errorf("ReadFrame with escapes = %v, want %v", frame, data)
	}
	if len(remaining) != 0 {
		t.Errorf("ReadFrame remaining = %v, want []", remaining)
	}
}
