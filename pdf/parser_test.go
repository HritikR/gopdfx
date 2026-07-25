package pdf

import (
	"bytes"
	"io"
	"testing"
)

func TestParser_Primitives(t *testing.T) {
	tests := []struct {
		input    string
		validate func(t *testing.T, obj Object)
	}{
		{"null", func(t *testing.T, obj Object) {
			if _, ok := obj.(Null); !ok {
				t.Errorf("Expected Null, got %T", obj)
			}
		}},
		{"true", func(t *testing.T, obj Object) {
			b, ok := AsBool(obj)
			if !ok || !b {
				t.Errorf("Expected true, got %v", obj)
			}
		}},
		{"false", func(t *testing.T, obj Object) {
			b, ok := AsBool(obj)
			if !ok || b {
				t.Errorf("Expected false, got %v", obj)
			}
		}},
		{"123", func(t *testing.T, obj Object) {
			i, ok := AsInt(obj)
			if !ok || i != 123 {
				t.Errorf("Expected integer 123, got %v", obj)
			}
		}},
		{"12.34", func(t *testing.T, obj Object) {
			f, ok := AsFloat(obj)
			if !ok || f != 12.34 {
				t.Errorf("Expected real 12.34, got %v", obj)
			}
		}},
		{"/MyName", func(t *testing.T, obj Object) {
			n, ok := AsName(obj)
			if !ok || n != "MyName" {
				t.Errorf("Expected name 'MyName', got %v", obj)
			}
		}},
		{"(Hello)", func(t *testing.T, obj Object) {
			s, ok := AsString(obj)
			if !ok || s != "Hello" {
				t.Errorf("Expected string 'Hello', got %v", obj)
			}
		}},
		{"<48656C6C6F>", func(t *testing.T, obj Object) {
			s, ok := AsString(obj)
			if !ok || s != "Hello" {
				t.Errorf("Expected hex string 'Hello', got %v", obj)
			}
		}},
	}

	for i, tc := range tests {
		l := NewLexer([]byte(tc.input))
		p := NewParser(l)
		obj, err := p.ParseObject()
		if err != nil {
			t.Fatalf("Test %d (%s) failed with error: %v", i, tc.input, err)
		}
		tc.validate(t, obj)
	}
}

func TestParser_IndirectReference(t *testing.T) {
	input := "12 0 R"
	l := NewLexer([]byte(input))
	p := NewParser(l)

	obj, err := p.ParseObject()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	ref, ok := AsRef(obj)
	if !ok {
		t.Fatalf("Expected IndirectRef, got %T", obj)
	}

	if ref.ObjectNumber != 12 || ref.GenerationNumber != 0 {
		t.Errorf("Expected 12 0 R, got %d %d R", ref.ObjectNumber, ref.GenerationNumber)
	}
}

func TestParser_IndirectObject(t *testing.T) {
	input := "12 0 obj\n(Hello World)\nendobj"
	l := NewLexer([]byte(input))
	p := NewParser(l)

	obj, err := p.ParseObject()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	iobj, ok := AsIndirectObj(obj)
	if !ok {
		t.Fatalf("Expected IndirectObj, got %T", obj)
	}

	if iobj.ObjectNumber != 12 || iobj.GenerationNumber != 0 {
		t.Errorf("Expected obj 12 0, got %d %d", iobj.ObjectNumber, iobj.GenerationNumber)
	}

	valStr, ok := AsString(iobj.Value)
	if !ok || valStr != "Hello World" {
		t.Errorf("Expected inner string 'Hello World', got %v", iobj.Value)
	}
}

func TestParser_ArrayAndDictionary(t *testing.T) {
	input := "[ 123 /Name << /Key (Value) >> ]"
	l := NewLexer([]byte(input))
	p := NewParser(l)

	obj, err := p.ParseObject()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	arr, ok := AsArray(obj)
	if !ok {
		t.Fatalf("Expected Array, got %T", obj)
	}

	if len(arr) != 3 {
		t.Fatalf("Expected array of length 3, got %d", len(arr))
	}

	iVal, ok := AsInt(arr[0])
	if !ok || iVal != 123 {
		t.Errorf("Expected arr[0] to be 123, got %v", arr[0])
	}

	nVal, ok := AsName(arr[1])
	if !ok || nVal != "Name" {
		t.Errorf("Expected arr[1] to be Name, got %v", arr[1])
	}

	dict, ok := AsDict(arr[2])
	if !ok {
		t.Fatalf("Expected arr[2] to be Dictionary, got %T", arr[2])
	}

	val, ok := dict[Name("Key")]
	if !ok {
		t.Fatalf("Expected key 'Key' in dictionary")
	}
	sVal, ok := AsString(val)
	if !ok || sVal != "Value" {
		t.Errorf("Expected Key value to be 'Value', got %v", val)
	}
}

func TestParser_Streams(t *testing.T) {
	t.Run("StreamWithLength", func(t *testing.T) {
		// Length is 5
		input := "10 0 obj\n<< /Length 5 >>\nstream\n12345\nendstream\nendobj"
		l := NewLexer([]byte(input))
		p := NewParser(l)

		obj, err := p.ParseObject()
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		iobj, ok := AsIndirectObj(obj)
		if !ok {
			t.Fatalf("Expected IndirectObj, got %T", obj)
		}

		stream, ok := AsStream(iobj.Value)
		if !ok {
			t.Fatalf("Expected Stream, got %T", iobj.Value)
		}

		if !bytes.Equal(stream.Data, []byte("12345")) {
			t.Errorf("Expected stream data '12345', got %q", stream.Data)
		}
	})

	t.Run("StreamWithLengthAndCRLF", func(t *testing.T) {
		// Using CRLF before and after stream
		input := "10 0 obj\r\n<< /Length 5 >>\r\nstream\r\n12345\r\nendstream\r\nendobj"
		l := NewLexer([]byte(input))
		p := NewParser(l)

		obj, err := p.ParseObject()
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		iobj, ok := AsIndirectObj(obj)
		if !ok {
			t.Fatalf("Expected IndirectObj, got %T", obj)
		}

		stream, ok := AsStream(iobj.Value)
		if !ok {
			t.Fatalf("Expected Stream, got %T", iobj.Value)
		}

		if !bytes.Equal(stream.Data, []byte("12345")) {
			t.Errorf("Expected stream data '12345', got %q", stream.Data)
		}
	})

	t.Run("StreamScanningFallback", func(t *testing.T) {
		// Length is dynamic/indirect reference, so we scan for endstream
		input := "10 0 obj\n<< /Length 12 0 R >>\nstream\nabcdef\nghijk\nendstream\nendobj"
		l := NewLexer([]byte(input))
		p := NewParser(l)

		obj, err := p.ParseObject()
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		iobj, ok := AsIndirectObj(obj)
		if !ok {
			t.Fatalf("Expected IndirectObj, got %T", obj)
		}

		stream, ok := AsStream(iobj.Value)
		if !ok {
			t.Fatalf("Expected Stream, got %T", iobj.Value)
		}

		expectedData := []byte("abcdef\nghijk")
		if !bytes.Equal(stream.Data, expectedData) {
			t.Errorf("Expected stream data %q, got %q", expectedData, stream.Data)
		}
	})
}

func TestParser_EOFAndErrors(t *testing.T) {
	l := NewLexer([]byte(""))
	p := NewParser(l)
	_, err := p.ParseObject()
	if err != io.EOF {
		t.Errorf("Expected io.EOF, got %v", err)
	}

	// Malformed syntax
	lMalformed := NewLexer([]byte("[ 1 2 3"))
	pMalformed := NewParser(lMalformed)
	_, err = pMalformed.ParseObject()
	if err == nil {
		t.Errorf("Expected error for unmatched array brackets, got nil")
	}
}
