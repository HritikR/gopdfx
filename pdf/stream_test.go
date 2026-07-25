package pdf

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"testing"
)

func TestStream_DecodeSingle(t *testing.T) {
	// Flate compressed "Stream content"
	rawText := []byte("Stream content")
	var compBuf bytes.Buffer
	zw := zlib.NewWriter(&compBuf)
	zw.Write(rawText)
	zw.Close()

	stream := &Stream{
		Dict: Dictionary{
			Name("Filter"): Name("FlateDecode"),
		},
		Data: compBuf.Bytes(),
	}

	decoded, err := stream.Decode()
	if err != nil {
		t.Fatalf("Stream decode failed: %v", err)
	}
	if !bytes.Equal(decoded, rawText) {
		t.Errorf("Expected %q, got %q", rawText, decoded)
	}
}

func TestStream_DecodeChained(t *testing.T) {
	// Chained filter: FlateDecode then ASCII85Decode
	// Wait, to decode it, ASCII85 is decoded first, then Flate.
	// We compress "Hello Chained Filters" with zlib, then encode with ASCII85.
	// But since we already have ASCII85 decode test, we can just do zlib compression + ASCIIHex encode:
	// Raw text: "Hello Chained"
	rawText := []byte("Hello Chained")
	var compBuf bytes.Buffer
	zw := zlib.NewWriter(&compBuf)
	zw.Write(rawText)
	zw.Close()

	// ASCIIHex encode the compressed bytes manually
	hexText := fmt.Sprintf("%X>", compBuf.Bytes())

	stream := &Stream{
		Dict: Dictionary{
			Name("Filter"): Array{Name("FlateDecode"), Name("ASCIIHexDecode")},
		},
		Data: []byte(hexText),
	}

	decoded, err := stream.Decode()
	if err != nil {
		t.Fatalf("Stream decode failed: %v", err)
	}
	if !bytes.Equal(decoded, rawText) {
		t.Errorf("Expected %q, got %q", rawText, decoded)
	}
}

func TestStream_CompressedXrefStreamIntegration(t *testing.T) {
	header := "%PDF-1.5\n"
	obj3 := "3 0 obj\n(Hello World)\nendobj\n"
	offset3 := len(header)

	// Object 6 is Xref Stream
	offset6 := offset3 + len(obj3)

	xrefStreamHeader := "6 0 obj\n<< /Type /XRef /Size 7 /W [1 2 1] /Filter /FlateDecode /Root 3 0 R >>\nstream\n"
	xrefStreamData := []byte{
		0x00, 0x00, 0x00, 0x00, // 0 free
		0x00, 0x00, 0x00, 0x00, // 1 free
		0x00, 0x00, 0x00, 0x00, // 2 free
		0x01, 0x00, byte(offset3), 0x00, // 3 normal
		0x00, 0x00, 0x00, 0x00, // 4 free
		0x00, 0x00, 0x00, 0x00, // 5 free
		0x01, 0x00, byte(offset6), 0x00, // 6 normal
	}

	// Compress the Xref Stream data
	var compBuf bytes.Buffer
	zw := zlib.NewWriter(&compBuf)
	zw.Write(xrefStreamData)
	zw.Close()

	xrefStreamTrailer := "\nendstream\nendobj\n"

	startxrefSection := fmt.Sprintf("startxref\n%d\n%%EOF", offset6)

	var pdfBytes []byte
	pdfBytes = append(pdfBytes, []byte(header)...)
	pdfBytes = append(pdfBytes, []byte(obj3)...)
	pdfBytes = append(pdfBytes, []byte(xrefStreamHeader)...)
	pdfBytes = append(pdfBytes, compBuf.Bytes()...)
	pdfBytes = append(pdfBytes, []byte(xrefStreamTrailer)...)
	pdfBytes = append(pdfBytes, []byte(startxrefSection)...)

	doc, err := OpenBytes(pdfBytes)
	if err != nil {
		t.Fatalf("OpenBytes failed on compressed xref stream: %v", err)
	}

	// Resolve Obj 3: should succeed
	obj3Resolved, err := doc.Resolve(IndirectRef{ObjectNumber: 3, GenerationNumber: 0})
	if err != nil {
		t.Fatalf("Resolve obj 3 failed: %v", err)
	}

	valStr, ok := AsString(obj3Resolved)
	if !ok || valStr != "Hello World" {
		t.Errorf("Expected string 'Hello World', got %v", obj3Resolved)
	}
}

func TestStream_CompressedObjectStreamIntegration(t *testing.T) {
	// Construct a PDF containing a compressed Object Stream (obj 3) using FlateDecode
	// Object Stream contains:
	// - Object 4: (Hello Compressed Object Stream)
	header := "%PDF-1.5\n"

	objStreamData := []byte("4 0     (Hello Compressed Object Stream)")
	var compObjBuf bytes.Buffer
	zw := zlib.NewWriter(&compObjBuf)
	zw.Write(objStreamData)
	zw.Close()

	obj3 := fmt.Sprintf("3 0 obj\n<< /Type /ObjStm /N 1 /First 8 /Filter /FlateDecode /Length %d >>\nstream\n", compObjBuf.Len())
	obj3Bytes := append([]byte(obj3), compObjBuf.Bytes()...)
	obj3Bytes = append(obj3Bytes, []byte("\nendstream\nendobj\n")...)

	offset3 := len(header)
	offset6 := offset3 + len(obj3Bytes)

	xrefStreamHeader := "6 0 obj\n<< /Type /XRef /Size 7 /W [1 2 1] /Root 3 0 R >>\nstream\n"
	xrefStreamData := []byte{
		0x00, 0x00, 0x00, 0x00, // 0 free
		0x00, 0x00, 0x00, 0x00, // 1 free
		0x00, 0x00, 0x00, 0x00, // 2 free
		0x01, 0x00, byte(offset3), 0x00, // 3 normal
		0x02, 0x00, 0x03, 0x00, // 4 compressed
		0x00, 0x00, 0x00, 0x00, // 5 free
		0x01, 0x00, byte(offset6), 0x00, // 6 normal
	}

	xrefStreamTrailer := "\nendstream\nendobj\n"
	startxrefSection := fmt.Sprintf("startxref\n%d\n%%EOF", offset6)

	var pdfBytes []byte
	pdfBytes = append(pdfBytes, []byte(header)...)
	pdfBytes = append(pdfBytes, obj3Bytes...)
	pdfBytes = append(pdfBytes, []byte(xrefStreamHeader)...)
	pdfBytes = append(pdfBytes, xrefStreamData...)
	pdfBytes = append(pdfBytes, []byte(xrefStreamTrailer)...)
	pdfBytes = append(pdfBytes, []byte(startxrefSection)...)

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
	if !ok || strVal != "Hello Compressed Object Stream" {
		t.Errorf("Expected 'Hello Compressed Object Stream', got %v", obj4)
	}
}
