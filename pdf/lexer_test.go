package pdf

import (
	"testing"
)

func TestLexer_BasicTokens(t *testing.T) {
	input := []byte("null true false [ ] << >> /Name 123 +456 -789 12.34 .5 -0.6")
	l := NewLexer(input)

	expected := []struct {
		typ tokenType
		val string
	}{
		{tokNull, "null"},
		{tokBool, "true"},
		{tokBool, "false"},
		{tokLeftBracket, "["},
		{tokRightBracket, "]"},
		{tokLeftDict, "<<"},
		{tokRightDict, ">>"},
		{tokName, "Name"},
		{tokInteger, "123"},
		{tokInteger, "+456"},
		{tokInteger, "-789"},
		{tokReal, "12.34"},
		{tokReal, ".5"},
		{tokReal, "-0.6"},
	}

	for i, tc := range expected {
		tok, err := l.NextToken()
		if err != nil {
			t.Fatalf("Test %d failed: unexpected error: %v", i, err)
		}
		if tok.typ != tc.typ {
			t.Errorf("Test %d failed: expected type %s, got %s", i, tc.typ, tok.typ)
		}
		if tok.val != tc.val {
			t.Errorf("Test %d failed: expected value '%s', got '%s'", i, tc.val, tok.val)
		}
	}

	tok, err := l.NextToken()
	if err != nil {
		t.Fatalf("Unexpected error at EOF: %v", err)
	}
	if tok.typ != tokEOF {
		t.Errorf("Expected EOF, got %s", tok.typ)
	}
}

func TestLexer_CommentsAndWhitespaces(t *testing.T) {
	input := []byte("  \t\r\n % comment here \r\n 123 % another comment\n/Name")
	l := NewLexer(input)

	tok1, err := l.NextToken()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if tok1.typ != tokInteger || tok1.val != "123" {
		t.Errorf("Expected integer 123, got %s (%s)", tok1.typ, tok1.val)
	}

	tok2, err := l.NextToken()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if tok2.typ != tokName || tok2.val != "Name" {
		t.Errorf("Expected name Name, got %s (%s)", tok2.typ, tok2.val)
	}
}

func TestLexer_NameEscapes(t *testing.T) {
	input := []byte("/A#20B /#23Sharp /#41 /SimpleName")
	l := NewLexer(input)

	expected := []string{"A B", "#Sharp", "A", "SimpleName"}

	for i, exp := range expected {
		tok, err := l.NextToken()
		if err != nil {
			t.Fatalf("Test %d failed: unexpected error: %v", i, err)
		}
		if tok.typ != tokName {
			t.Errorf("Test %d failed: expected Name, got %s", i, tok.typ)
		}
		if tok.val != exp {
			t.Errorf("Test %d failed: expected '%s', got '%s'", i, exp, tok.val)
		}
	}
}

func TestLexer_LiteralStrings(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"(Hello, World!)", "Hello, World!"},
		{"(Hello \\(nested\\) World)", "Hello (nested) World"},
		{"(Line 1\\nLine 2\\rTab\\tBack\\bFF\\fSlash\\\\)", "Line 1\nLine 2\rTab\tBack\bFF\fSlash\\"},
		{"(Octal \\101 \\123)", "Octal A S"},            // \101 is 'A', \123 is 'S'
		{"(OctalShort \\1 \\12)", "OctalShort \x01 \n"}, // \1 is 0x01, \12 is 0x0a ('\n')
		{"(Line\\\nContinuation)", "LineContinuation"},
		{"(Line\\\r\nContinuation)", "LineContinuation"},
		{"(Nested (Parentheses) Test)", "Nested (Parentheses) Test"},
	}

	for i, tc := range tests {
		l := NewLexer([]byte(tc.input))
		tok, err := l.NextToken()
		if err != nil {
			t.Fatalf("Test %d (%s) failed with error: %v", i, tc.input, err)
		}
		if tok.typ != tokString {
			t.Errorf("Test %d failed: expected String token, got %s", i, tok.typ)
		}
		if tok.val != tc.expected {
			t.Errorf("Test %d failed:\nexpected: %q\ngot:      %q", i, tc.expected, tok.val)
		}
	}
}

func TestLexer_HexStrings(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"<4E6F76656D626572>", "November"},
		{"<4e6f>", "No"},
		{"<4e 6f>", "No"}, // whitespace ignored
		{"<4>", "@"},      // padded to <40> which is '@'
		{"<>", ""},
	}

	for i, tc := range tests {
		l := NewLexer([]byte(tc.input))
		tok, err := l.NextToken()
		if err != nil {
			t.Fatalf("Test %d failed: %v", i, err)
		}
		if tok.typ != tokHexString {
			t.Errorf("Test %d failed: expected HexString token, got %s", i, tok.typ)
		}
		if tok.val != tc.expected {
			t.Errorf("Test %d failed: expected %q, got %q", i, tc.expected, tok.val)
		}
	}
}

func TestLexer_KeywordsAndOperators(t *testing.T) {
	input := []byte("obj endobj stream endstream R xref trailer startxref Tj TJ BT ET cm")
	l := NewLexer(input)

	expected := []struct {
		typ tokenType
		val string
	}{
		{tokObj, "obj"},
		{tokEndObj, "endobj"},
		{tokStream, "stream"},
		{tokEndStream, "endstream"},
		{tokRef, "R"},
		{tokXref, "xref"},
		{tokTrailer, "trailer"},
		{tokStartXref, "startxref"},
		{tokKeyword, "Tj"},
		{tokKeyword, "TJ"},
		{tokKeyword, "BT"},
		{tokKeyword, "ET"},
		{tokKeyword, "cm"},
	}

	for i, tc := range expected {
		tok, err := l.NextToken()
		if err != nil {
			t.Fatalf("Test %d failed: %v", i, err)
		}
		if tok.typ != tc.typ {
			t.Errorf("Test %d failed: expected %s, got %s", i, tc.typ, tok.typ)
		}
		if tok.val != tc.val {
			t.Errorf("Test %d failed: expected '%s', got '%s'", i, tc.val, tok.val)
		}
	}
}
