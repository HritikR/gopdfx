package pdf

import (
	"fmt"
	"strings"
	"testing"
)

func TestDocument_ClassicResolve(t *testing.T) {
	header := "%PDF-1.4\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 0 >>\nendobj\n"

	offset1 := len(header)
	offset2 := len(header) + len(obj1)

	xrefTable := fmt.Sprintf("xref\n0 3\n0000000000 65535 f \n%010d 00000 n \n%010d 00000 n \n", offset1, offset2)
	offsetXref := len(header) + len(obj1) + len(obj2)

	trailer := fmt.Sprintf("trailer\n<< /Size 3 /Root 1 0 R >>\nstartxref\n%d\n%%EOF", offsetXref)

	pdfBytes := []byte(header + obj1 + obj2 + xrefTable + trailer)

	doc, err := OpenBytes(pdfBytes)
	if err != nil {
		t.Fatalf("OpenBytes failed: %v", err)
	}

	// 1. Resolve Catalog (1 0 R)
	catObj, err := doc.Resolve(IndirectRef{ObjectNumber: 1, GenerationNumber: 0})
	if err != nil {
		t.Fatalf("Resolve catalog failed: %v", err)
	}
	catDict, ok := AsDict(catObj)
	if !ok {
		t.Fatalf("Expected Catalog to be Dictionary, got %T", catObj)
	}
	typeVal, _ := AsName(catDict[Name("Type")])
	if typeVal != "Catalog" {
		t.Errorf("Expected Catalog Type, got %s", typeVal)
	}

	// 2. Resolve via Catalog() helper
	catalogDict, err := doc.Catalog()
	if err != nil {
		t.Fatalf("Catalog() failed: %v", err)
	}
	if catalogDict == nil {
		t.Fatal("Expected Catalog dict, got nil")
	}

	// 3. Cache Check: Modify cache and ensure Resolve returns cached modified version
	doc.objectCache[1] = Boolean(true)
	cachedObj, err := doc.Resolve(IndirectRef{ObjectNumber: 1, GenerationNumber: 0})
	if err != nil {
		t.Fatalf("Resolve cached failed: %v", err)
	}
	if bVal, ok := AsBool(cachedObj); !ok || !bVal {
		t.Errorf("Expected cached boolean value, got %v", cachedObj)
	}
}

func TestDocument_XrefPrevCycle(t *testing.T) {
	doc := &Document{input: []byte{0}}
	err := doc.loadXrefAndTrailerVisited(0, map[int64]struct{}{0: {}})
	if err == nil {
		t.Fatal("expected an error for a cyclic xref /Prev chain")
	}
	if !strings.Contains(err.Error(), "cycle detected") {
		t.Fatalf("expected cycle error, got %v", err)
	}
}

func TestDocument_ObjectStreamResolve(t *testing.T) {
	// Construct a PDF containing an uncompressed Object Stream (obj 3)
	// Object Stream contains:
	// - Object 4: (Hello)
	// - Object 5: [1 2 3]
	header := "%PDF-1.5\n"
	obj3 := "3 0 obj\n<< /Type /ObjStm /N 2 /First 12 /Length 27 >>\nstream\n4 0 5 8     (Hello) [1 2 3]\nendstream\nendobj\n"

	offset3 := len(header)

	// Xref table lists:
	// Object 3: Normal at offset3
	// Object 4: Compressed in Object Stream 3 at index 0
	// Object 5: Compressed in Object Stream 3 at index 1
	// Wait, since classic xref table doesn't support Type 2 entries, we use an Xref Stream!
	// Let's build the Xref Stream (obj 6)
	// Dict: /Type /XRef /Size 7 /W [1 2 1] /Index [0 7]
	// Entries:
	// obj 0: Free
	// obj 1: Free
	// obj 2: Free
	// obj 3: Normal (offset3 = 9) -> [0x01, 0x00, 0x09, 0x00]
	// obj 4: Compressed (objStm 3, idx 0) -> [0x02, 0x00, 0x03, 0x00]
	// obj 5: Compressed (objStm 3, idx 1) -> [0x02, 0x00, 0x03, 0x01]
	// obj 6: Normal (Xref Stream offset) -> we can resolve offset6

	// Let's write the xref stream data
	// Length of obj3 is len(obj3) = 111 bytes.
	// Offset of obj6 (Xref Stream) = offset3 + len(obj3) = 9 + 111 = 120.
	offset6 := offset3 + len(obj3)

	xrefStreamHeader := "6 0 obj\n<< /Type /XRef /Size 7 /W [1 2 1] /Length 28 /Root 3 0 R >>\nstream\n"
	xrefStreamData := []byte{
		0x00, 0x00, 0x00, 0x00, // 0 free
		0x00, 0x00, 0x00, 0x00, // 1 free
		0x00, 0x00, 0x00, 0x00, // 2 free
		0x01, 0x00, byte(offset3), 0x00, // 3 normal
		0x02, 0x00, 0x03, 0x00, // 4 compressed
		0x02, 0x00, 0x03, 0x01, // 5 compressed
		0x01, 0x00, byte(offset6), 0x00, // 6 normal
	}
	xrefStreamTrailer := "\nendstream\nendobj\n"

	startxrefOffset := offset6 + len(xrefStreamHeader) + len(xrefStreamData) + len(xrefStreamTrailer)

	startxrefSection := fmt.Sprintf("startxref\n%d\n%%EOF", offset6)

	var pdfBytes []byte
	pdfBytes = append(pdfBytes, []byte(header)...)
	pdfBytes = append(pdfBytes, []byte(obj3)...)
	pdfBytes = append(pdfBytes, []byte(xrefStreamHeader)...)
	pdfBytes = append(pdfBytes, xrefStreamData...)
	pdfBytes = append(pdfBytes, []byte(xrefStreamTrailer)...)
	pdfBytes = append(pdfBytes, []byte(startxrefSection)...)

	_ = startxrefOffset // used implicitly

	doc, err := OpenBytes(pdfBytes)
	if err != nil {
		t.Fatalf("OpenBytes failed: %v", err)
	}

	// Resolve compressed Object 4
	obj4, err := doc.Resolve(IndirectRef{ObjectNumber: 4, GenerationNumber: 0})
	if err != nil {
		t.Fatalf("Resolve obj 4 failed: %v", err)
	}
	strVal, ok := AsString(obj4)
	if !ok || strVal != "Hello" {
		t.Errorf("Expected string 'Hello', got %v", obj4)
	}

	// Resolve compressed Object 5
	obj5, err := doc.Resolve(IndirectRef{ObjectNumber: 5, GenerationNumber: 0})
	if err != nil {
		t.Fatalf("Resolve obj 5 failed: %v", err)
	}
	arrVal, ok := AsArray(obj5)
	if !ok || len(arrVal) != 3 {
		t.Errorf("Expected array of length 3, got %v", obj5)
	}
}

func TestDocument_IncrementalUpdate(t *testing.T) {
	// Construct a PDF with two sections:
	// Section 1 (original, at offset 9): Obj 1 (Catalog) and Obj 2 (Pages)
	// Section 2 (incremental update, at offset 180): Obj 2 is updated to Count = 5
	header := "%PDF-1.4\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2Original := "2 0 obj\n<< /Type /Pages /Count 0 >>\nendobj\n"

	offset1 := len(header)
	offset2Original := len(header) + len(obj1)

	xref1 := fmt.Sprintf("xref\n0 3\n0000000000 65535 f \n%010d 00000 n \n%010d 00000 n \n", offset1, offset2Original)
	offsetXref1 := len(header) + len(obj1) + len(obj2Original)
	trailer1 := fmt.Sprint("trailer\n<< /Size 3 /Root 1 0 R >>\n")

	// incremental update starts here
	offsetIncremental := offsetXref1 + len(xref1) + len(trailer1)
	obj2Updated := "2 0 obj\n<< /Type /Pages /Count 5 >>\nendobj\n"

	offset2Updated := offsetIncremental

	xref2 := fmt.Sprintf("xref\n2 1\n%010d 00000 n \n", offset2Updated)
	offsetXref2 := offsetIncremental + len(obj2Updated)
	trailer2 := fmt.Sprintf("trailer\n<< /Size 3 /Root 1 0 R /Prev %d >>\nstartxref\n%d\n%%EOF", offsetXref1, offsetXref2)

	var pdfBytes []byte
	pdfBytes = append(pdfBytes, []byte(header)...)
	pdfBytes = append(pdfBytes, []byte(obj1)...)
	pdfBytes = append(pdfBytes, []byte(obj2Original)...)
	pdfBytes = append(pdfBytes, []byte(xref1)...)
	pdfBytes = append(pdfBytes, []byte(trailer1)...)
	pdfBytes = append(pdfBytes, []byte(obj2Updated)...)
	pdfBytes = append(pdfBytes, []byte(xref2)...)
	pdfBytes = append(pdfBytes, []byte(trailer2)...)

	doc, err := OpenBytes(pdfBytes)
	if err != nil {
		t.Fatalf("OpenBytes failed: %v", err)
	}

	// Resolve Obj 2: It should resolve to the updated version (Count = 5)
	pagesObj, err := doc.Resolve(IndirectRef{ObjectNumber: 2, GenerationNumber: 0})
	if err != nil {
		t.Fatalf("Resolve obj 2 failed: %v", err)
	}
	pagesDict, ok := AsDict(pagesObj)
	if !ok {
		t.Fatalf("Expected Pages to be Dictionary, got %T", pagesObj)
	}
	countVal, _ := AsInt(pagesDict[Name("Count")])
	if countVal != 5 {
		t.Errorf("Expected updated Count = 5, got %d", countVal)
	}
}
