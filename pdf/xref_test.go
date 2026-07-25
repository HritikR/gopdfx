package pdf

import (
	"testing"
)

func TestFindStartXrefOffset(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"startxref\n1234\n%%EOF", 1234},
		{"startxref\r\n5678\r\n%%EOF\r\n", 5678},
		{"some garbage bytes\nstartxref\n999\n%%EOF  \n\t ", 999},
	}

	for i, tc := range tests {
		offset, err := FindStartXrefOffset([]byte(tc.input))
		if err != nil {
			t.Fatalf("Test %d failed: %v", i, err)
		}
		if offset != tc.expected {
			t.Errorf("Test %d: expected offset %d, got %d", i, tc.expected, offset)
		}
	}
}

func TestParseXrefTable(t *testing.T) {
	input := `xref
0 2
0000000000 65535 f 
0000000015 00000 n 
10 3
0000000120 00000 n 
0000000200 00001 n 
0000000000 00002 f 
trailer
<< /Size 13 /Root 1 0 R >>`

	l := NewLexer([]byte(input))
	p := NewParser(l)

	entries, err := ParseXrefTable(p)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	expected := map[int64]XrefEntry{
		0:  {Type: XrefEntryFree, Offset: 0, GenerationNumber: 65535},
		1:  {Type: XrefEntryNormal, Offset: 15, GenerationNumber: 0},
		10: {Type: XrefEntryNormal, Offset: 120, GenerationNumber: 0},
		11: {Type: XrefEntryNormal, Offset: 200, GenerationNumber: 1},
		12: {Type: XrefEntryFree, Offset: 0, GenerationNumber: 2},
	}

	if len(entries) != len(expected) {
		t.Fatalf("Expected %d entries, got %d", len(expected), len(entries))
	}

	for objNum, exp := range expected {
		got, exists := entries[objNum]
		if !exists {
			t.Errorf("Missing entry for object %d", objNum)
			continue
		}
		if got.Type != exp.Type || got.Offset != exp.Offset || got.GenerationNumber != exp.GenerationNumber {
			t.Errorf("Object %d: expected %+v, got %+v", objNum, exp, got)
		}
	}

	// Verify parser position is at "trailer"
	tok, err := p.lexer.NextToken()
	if err != nil || tok.typ != tokTrailer {
		t.Errorf("Expected lexer to be at trailer, got %s (%v)", tok.val, err)
	}
}

func TestParseXrefStream(t *testing.T) {
	// Let's create an Xref Stream representation
	// W: [1 2 1] -> entry size = 4 bytes.
	// Index: [0 2 10 1]
	// Sub-section 1: object 0 and 1 (2 entries)
	// Sub-section 2: object 10 (1 entry)
	// Entries data bytes:
	// Entry 0 (obj 0): free, next free obj = 0, gen = 65535 -> bytes: [0x00, 0x00, 0x00, 0xFF, 0xFF] (wait, gen width is 1 so 65535 fits modulo 256 -> 0xFF)
	// Entry 1 (obj 1): normal, offset = 120 (0x0078), gen = 0 -> bytes: [0x01, 0x00, 0x78, 0x00]
	// Entry 2 (obj 10): compressed, stream obj = 2 (0x0002), index = 5 (0x05) -> bytes: [0x02, 0x00, 0x02, 0x05]
	
	streamDict := Dictionary{
		Name("Type"):  Name("XRef"),
		Name("Size"):  Integer(11),
		Name("W"):     Array{Integer(1), Integer(2), Integer(1)},
		Name("Index"): Array{Integer(0), Integer(2), Integer(10), Integer(1)},
	}

	data := []byte{
		0x00, 0x00, 0x00, 0xFF, // Free entry: type 0, offset 0, gen 255
		0x01, 0x00, 0x78, 0x00, // Normal entry: type 1, offset 120, gen 0
		0x02, 0x00, 0x02, 0x05, // Compressed entry: type 2, objStm 2, index 5
	}

	streamObj := &Stream{
		Dict: streamDict,
		Data: data,
	}

	entries, err := ParseXrefStream(streamObj)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	expected := map[int64]XrefEntry{
		0:  {Type: XrefEntryFree, Offset: 0, GenerationNumber: 255},
		1:  {Type: XrefEntryNormal, Offset: 120, GenerationNumber: 0},
		10: {Type: XrefEntryCompressed, Offset: 2, GenerationNumber: 5},
	}

	if len(entries) != len(expected) {
		t.Fatalf("Expected %d entries, got %d", len(expected), len(entries))
	}

	for objNum, exp := range expected {
		got, exists := entries[objNum]
		if !exists {
			t.Errorf("Missing entry for object %d", objNum)
			continue
		}
		if got.Type != exp.Type || got.Offset != exp.Offset || got.GenerationNumber != exp.GenerationNumber {
			t.Errorf("Object %d: expected %+v, got %+v", objNum, exp, got)
		}
	}
}
