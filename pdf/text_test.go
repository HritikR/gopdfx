package pdf

import (
	"fmt"
	"testing"
)

func TestText_CMapParsing(t *testing.T) {
	// A mock CMap stream
	cmapData := []byte(`/CIDInit /ProcSet findresource begin
12 dict begin
begincmap
/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def
/CMapName /Custom-ToUnicode def
/CMapType 2 def
1 begincodespacerange
<0000> <FFFF>
endcodespacerange
1 beginbfchar
<01> <0041>
endbfchar
1 beginbfrange
<02> <03> <0042>
endbfrange
endcmap
CMapName currentdict /CMap defineresource pop
end
end`)

	cmap, err := ParseCMap(cmapData)
	if err != nil {
		t.Fatalf("ParseCMap failed: %v", err)
	}

	// Test bfchar lookup
	val, ok := cmap.Lookup(1)
	if !ok || val != "A" {
		t.Errorf("Expected 'A' for code 1, got %q (ok: %t)", val, ok)
	}

	// Test bfrange lookup (should map 2 -> B, 3 -> C)
	val, ok = cmap.Lookup(2)
	if !ok || val != "B" {
		t.Errorf("Expected 'B' for code 2, got %q", val)
	}

	val, ok = cmap.Lookup(3)
	if !ok || val != "C" {
		t.Errorf("Expected 'C' for code 3, got %q", val)
	}
}

func TestText_ContentStreamParser(t *testing.T) {
	// Tj, TJ, ', "
	content := []byte(`BT
/F1 12 Tf
(Hello) Tj
[ (World) -120 (with) -50 (spacing) ] TJ
ET`)

	doc := &Document{}
	// Test without font mappings first (fallbacks directly to bytes)
	txt, err := doc.parseContentStream(content, nil)
	if err != nil {
		t.Fatalf("parseContentStream failed: %v", err)
	}

	// Hello + World (since -120 is spacing) + with (since -50 is not enough for a space) + spacing
	expected := "HelloWorld withspacing\n"
	if txt != expected {
		t.Errorf("Expected %q, got %q", expected, txt)
	}
}

func TestText_FullExtractionIntegration(t *testing.T) {
	// Construct a PDF document with:
	// - Obj 1 (Catalog) -> /Pages 2 0 R
	// - Obj 2 (Pages Tree Node) -> /Kids [ 3 0 R ] /Count 1
	// - Obj 3 (Page Leaf) -> /Parent 2 0 R /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >>
	// - Obj 4 (Contents Stream) -> "BT /F1 12 Tf (Parsed PDF Text) Tj ET"
	// - Obj 5 (Font Dict) -> /Type /Font /Subtype /Type1
	
	header := "%PDF-1.4\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [ 3 0 R ] /Count 1 >>\nendobj\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>\nendobj\n"
	
	contentStream := "BT /F1 10 Tf (Parsed PDF Text) Tj ET"
	obj4 := fmt.Sprintf("4 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(contentStream), contentStream)
	obj5 := "5 0 obj\n<< /Type /Font /Subtype /Type1 >>\nendobj\n"
	
	offset1 := len(header)
	offset2 := offset1 + len(obj1)
	offset3 := offset2 + len(obj2)
	offset4 := offset3 + len(obj3)
	offset5 := offset4 + len(obj4)
	offsetXref := offset5 + len(obj5)
	
	xrefTable := fmt.Sprintf("xref\n0 6\n0000000000 65535 f \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n", offset1, offset2, offset3, offset4, offset5)
	trailer := fmt.Sprintf("trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n%d\n%%EOF", offsetXref)
	
	pdfBytes := []byte(header + obj1 + obj2 + obj3 + obj4 + obj5 + xrefTable + trailer)
	
	doc, err := OpenBytes(pdfBytes)
	if err != nil {
		t.Fatalf("OpenBytes failed: %v", err)
	}
	
	numPages, err := doc.NumPages()
	if err != nil {
		t.Fatalf("NumPages failed: %v", err)
	}
	if numPages != 1 {
		t.Fatalf("Expected 1 page, got %d", numPages)
	}
	
	text, err := doc.Text()
	if err != nil {
		t.Fatalf("Text failed: %v", err)
	}
	
	expectedText := "Parsed PDF Text\n"
	if text != expectedText {
		t.Errorf("Expected extracted text %q, got %q", expectedText, text)
	}
}

func TestText_InheritedResources(t *testing.T) {
	// Construct a PDF document where the Font resources are inherited from the parent /Pages node
	// - Obj 1 (Catalog) -> /Pages 2 0 R
	// - Obj 2 (Pages Tree Node) -> /Kids [ 3 0 R ] /Count 1 /Resources << /Font << /F1 5 0 R >> >>
	// - Obj 3 (Page Leaf) -> /Parent 2 0 R /Contents 4 0 R (No /Resources dictionary here!)
	// - Obj 4 (Contents Stream) -> "BT /F1 10 Tf (Inherited Text) Tj ET"
	// - Obj 5 (Font Dict) -> /Type /Font /Subtype /Type1
	
	header := "%PDF-1.4\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [ 3 0 R ] /Count 1 /Resources << /Font << /F1 5 0 R >> >> >>\nendobj\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /Contents 4 0 R >>\nendobj\n"
	
	contentStream := "BT /F1 10 Tf (Inherited Text) Tj ET"
	obj4 := fmt.Sprintf("4 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(contentStream), contentStream)
	obj5 := "5 0 obj\n<< /Type /Font /Subtype /Type1 >>\nendobj\n"
	
	offset1 := len(header)
	offset2 := offset1 + len(obj1)
	offset3 := offset2 + len(obj2)
	offset4 := offset3 + len(obj3)
	offset5 := offset4 + len(obj4)
	offsetXref := offset5 + len(obj5)
	
	xrefTable := fmt.Sprintf("xref\n0 6\n0000000000 65535 f \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n", offset1, offset2, offset3, offset4, offset5)
	trailer := fmt.Sprintf("trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n%d\n%%EOF", offsetXref)
	
	pdfBytes := []byte(header + obj1 + obj2 + obj3 + obj4 + obj5 + xrefTable + trailer)
	
	doc, err := OpenBytes(pdfBytes)
	if err != nil {
		t.Fatalf("OpenBytes failed: %v", err)
	}
	
	text, err := doc.Text()
	if err != nil {
		t.Fatalf("Text failed: %v", err)
	}
	
	expectedText := "Inherited Text\n"
	if text != expectedText {
		t.Errorf("Expected extracted text %q, got %q", expectedText, text)
	}
}
