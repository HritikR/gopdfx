package pdf

import (
	"bytes"
	"fmt"
	"strconv"
)

// FindStartXrefOffset scans the PDF file backward to find the 'startxref' offset.
func FindStartXrefOffset(data []byte) (int64, error) {
	idx := bytes.LastIndex(data, []byte("startxref"))
	if idx == -1 {
		return 0, fmt.Errorf("%w: could not find 'startxref' keyword", ErrInvalidPDF)
	}

	// Initialize lexer at "startxref"
	subLexer := NewLexer(data[idx:])
	tok, err := subLexer.NextToken()
	if err != nil {
		return 0, fmt.Errorf("%w: failed to read startxref token: %v", ErrInvalidPDF, err)
	}
	if tok.typ != tokStartXref {
		return 0, fmt.Errorf("%w: expected 'startxref' keyword, got '%s'", ErrInvalidPDF, tok.val)
	}

	tokOffset, err := subLexer.NextToken()
	if err != nil {
		return 0, fmt.Errorf("%w: failed to read offset after startxref: %v", ErrInvalidPDF, err)
	}
	if tokOffset.typ != tokInteger {
		return 0, fmt.Errorf("%w: expected integer offset after startxref, got '%s'", ErrInvalidPDF, tokOffset.val)
	}

	offset, err := strconv.ParseInt(tokOffset.val, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: invalid startxref offset '%s': %v", ErrInvalidPDF, tokOffset.val, err)
	}

	return offset, nil
}

// ParseTrailerDict parses the trailer dictionary following the classic xref table.
func ParseTrailerDict(p *Parser) (Dictionary, error) {
	tok, err := p.lexer.NextToken()
	if err != nil {
		return nil, fmt.Errorf("%w: failed to read trailer token: %v", ErrCorruptObject, err)
	}
	if tok.typ != tokTrailer {
		return nil, fmt.Errorf("%w: expected 'trailer' keyword, got '%s'", ErrCorruptObject, tok.val)
	}

	tokDict, err := p.lexer.NextToken()
	if err != nil {
		return nil, fmt.Errorf("%w: failed to read dictionary after trailer: %v", ErrCorruptObject, err)
	}
	if tokDict.typ != tokLeftDict {
		return nil, fmt.Errorf("%w: expected dictionary start '<<' after trailer, got '%s'", ErrCorruptObject, tokDict.val)
	}

	return p.parseDictionary()
}
