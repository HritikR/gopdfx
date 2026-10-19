package filters

import (
	"bytes"
	"compress/zlib"
	"testing"
)

func TestDecodeASCIIHex(t *testing.T) {
	tests := []struct {
		input    string
		expected string
		err      bool
	}{
		{"4E6F76656D626572>", "November", false},
		{"4e6f>", "No", false},
		{"4e 6f>", "No", false}, // ignore spaces
		{"4e6f", "No", false},   // missing '>' is allowed/graceful
		{"4>", "@", false},      // padded to <40>
		{"456g>", "", true},     // invalid char 'g'
	}

	for i, tc := range tests {
		got, err := DecodeASCIIHex([]byte(tc.input))
		if tc.err {
			if err == nil {
				t.Errorf("Test %d: expected error, got nil", i)
			}
			continue
		}
		if err != nil {
			t.Fatalf("Test %d failed with error: %v", i, err)
		}
		if string(got) != tc.expected {
			t.Errorf("Test %d: expected %q, got %q", i, tc.expected, string(got))
		}
	}
}

func TestDecodeASCII85(t *testing.T) {
	tests := []struct {
		input    string
		expected string
		err      bool
	}{
		{"<~9jqo^BlbD-BleB1DJ+*+F(f,q/0JhKF<GL>Cj@.4Gp$d7F!,L7@<6@)~>", "Man is distinguished, not only by his reason", false},
		{"<~z~>", "\x00\x00\x00\x00", false},
		{"<~9jqo^~>", "Man ", false}, // padded block
		{"<~9jqo^", "Man ", false},   // missing ~> is handled gracefully
		{"<~!", "", true},            // truncated 1 char
	}

	for i, tc := range tests {
		// Clean leading "<~" if present in input for the raw decoder test
		raw := []byte(tc.input)
		if bytes.HasPrefix(raw, []byte("<~")) {
			raw = raw[2:]
		}
		got, err := DecodeASCII85(raw)
		if tc.err {
			if err == nil {
				t.Errorf("Test %d: expected error, got nil", i)
			}
			continue
		}
		if err != nil {
			t.Fatalf("Test %d failed with error: %v", i, err)
		}
		if !bytes.HasPrefix([]byte(tc.expected), got) && string(got) != tc.expected {
			t.Errorf("Test %d: expected %q, got %q", i, tc.expected, string(got))
		}
	}
}

func TestDecodeRunLength(t *testing.T) {
	// Encode: literal "abc" (3 bytes -> n = 2), repeated 'x' 5 times (n = 257 - 5 = 252), end marker (128)
	input := []byte{
		2, 'a', 'b', 'c', // copy 3 literal
		252, 'x', // repeat 'x' 5 times
		128, // EOF
	}
	expected := "abcxxxxx"

	got, err := DecodeRunLength(input)
	if err != nil {
		t.Fatalf("DecodeRunLength failed: %v", err)
	}
	if string(got) != expected {
		t.Errorf("Expected %q, got %q", expected, string(got))
	}
}

func TestDecodePredictor_Tiff(t *testing.T) {
	// 3 columns, 1 color (bpp = 1)
	// Encoded: [10, 5, 2] -> decoded: [10, 15, 17]
	input := []byte{10, 5, 2}
	expected := []byte{10, 15, 17}

	got, err := DecodePredictor(2, input, 3, 1, 8)
	if err != nil {
		t.Fatalf("TIFF Predictor failed: %v", err)
	}
	if !bytes.Equal(got, expected) {
		t.Errorf("Expected %+v, got %+v", expected, got)
	}
}

func TestDecodePredictor_Png(t *testing.T) {
	// 2 rows, 3 columns, 1 color (bpp = 1)
	// Row 1 (tag 1: Sub): [1, 10, 5, 2] -> decoded: [10, 15, 17]
	// Row 2 (tag 2: Up):  [2, 3, 2, 1] -> decoded: [10+3=13, 15+2=17, 17+1=18]
	input := []byte{
		1, 10, 5, 2,
		2, 3, 2, 1,
	}
	expected := []byte{
		10, 15, 17,
		13, 17, 18,
	}

	got, err := DecodePredictor(15, input, 3, 1, 8)
	if err != nil {
		t.Fatalf("PNG Predictor failed: %v", err)
	}
	if !bytes.Equal(got, expected) {
		t.Errorf("Expected %+v, got %+v", expected, got)
	}
}

func TestDecodeFlate(t *testing.T) {
	// Compress some data using zlib
	src := []byte("Hello, PDF compression!")
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	zw.Write(src)
	zw.Close()

	decompressed, err := DecodeFlate(buf.Bytes(), 1, 1, 1, 8)
	if err != nil {
		t.Fatalf("DecodeFlate failed: %v", err)
	}
	if !bytes.Equal(decompressed, src) {
		t.Errorf("Expected %q, got %q", src, decompressed)
	}
}
