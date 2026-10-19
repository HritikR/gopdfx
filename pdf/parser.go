package pdf

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
)

type Parser struct {
	lexer *Lexer
}

func NewParser(lexer *Lexer) *Parser {
	return &Parser{lexer: lexer}
}

// ParseAt seeks to the given offset and parses the PDF object at that location.
func (p *Parser) ParseAt(offset int64) (Object, error) {
	p.lexer.SetPos(int(offset))
	return p.ParseObject()
}

// ParseObject parses the next PDF object from the token stream.
func (p *Parser) ParseObject() (Object, error) {
	tok, err := p.lexer.NextToken()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCorruptObject, err)
	}
	if tok.typ == tokEOF {
		return nil, io.EOF
	}
	return p.parseObjectWithToken(tok)
}

func (p *Parser) parseObjectWithToken(tok token) (Object, error) {
	switch tok.typ {
	case tokNull:
		return Null{}, nil
	case tokBool:
		return Boolean(tok.val == "true"), nil
	case tokInteger:
		// Lookahead to check if this is an indirect reference (e.g. 12 0 R)
		// or indirect object definition (e.g. 12 0 obj)
		savedPos := p.lexer.Pos()

		tok2, err := p.lexer.NextToken()
		if err == nil && tok2.typ == tokInteger {
			tok3, err := p.lexer.NextToken()
			if err == nil {
				if tok3.typ == tokRef {
					objNum, _ := strconv.ParseInt(tok.val, 10, 64)
					genNum, _ := strconv.ParseInt(tok2.val, 10, 64)
					return IndirectRef{ObjectNumber: objNum, GenerationNumber: genNum}, nil
				}
				if tok3.typ == tokObj {
					objNum, _ := strconv.ParseInt(tok.val, 10, 64)
					genNum, _ := strconv.ParseInt(tok2.val, 10, 64)

					// Parse the value inside the indirect object
					val, err := p.ParseObject()
					if err != nil {
						return nil, err
					}

					// A dictionary could immediately be followed by stream data
					if dict, ok := val.(Dictionary); ok {
						savedPosAfterDict := p.lexer.Pos()
						tok4, err := p.lexer.NextToken()
						if err == nil && tok4.typ == tokStream {
							// It is a stream!
							streamObj, err := p.parseStreamData(dict)
							if err != nil {
								return nil, err
							}
							val = streamObj
						} else {
							// Not a stream, backtrack to after the dictionary value
							p.lexer.SetPos(savedPosAfterDict)
						}
					}

					// Expect "endobj"
					tokEnd, err := p.lexer.NextToken()
					if err != nil {
						return nil, fmt.Errorf("%w: failed parsing endobj: %v", ErrCorruptObject, err)
					}
					if tokEnd.typ != tokEndObj {
						return nil, fmt.Errorf("%w: expected 'endobj', got '%s' at position %d", ErrCorruptObject, tokEnd.val, p.lexer.Pos())
					}

					return &IndirectObj{
						ObjectNumber:     objNum,
						GenerationNumber: genNum,
						Value:            val,
					}, nil
				}
			}
		}

		// Backtrack if not a reference or indirect object definition
		p.lexer.SetPos(savedPos)
		val, _ := strconv.ParseInt(tok.val, 10, 64)
		return Integer(val), nil

	case tokReal:
		val, _ := strconv.ParseFloat(tok.val, 64)
		return Real(val), nil

	case tokName:
		return Name(tok.val), nil

	case tokString:
		return StringObj{Value: tok.val, Hex: false}, nil

	case tokHexString:
		return StringObj{Value: tok.val, Hex: true}, nil

	case tokLeftBracket:
		return p.parseArray()

	case tokLeftDict:
		dict, err := p.parseDictionary()
		if err != nil {
			return nil, err
		}

		// Check if it's a stream outside of an indirect object
		savedPos := p.lexer.Pos()
		tokNext, err := p.lexer.NextToken()
		if err == nil && tokNext.typ == tokStream {
			return p.parseStreamData(dict)
		}
		p.lexer.SetPos(savedPos)

		return dict, nil

	default:
		return nil, fmt.Errorf("%w: unexpected token type '%s' (value: '%s') at position %d", ErrCorruptObject, tok.typ, tok.val, p.lexer.Pos())
	}
}

func (p *Parser) parseArray() (Array, error) {
	var arr Array
	for {
		savedPos := p.lexer.Pos()
		tok, err := p.lexer.NextToken()
		if err != nil {
			return nil, fmt.Errorf("%w: failed inside array: %v", ErrCorruptObject, err)
		}
		if tok.typ == tokRightBracket {
			break
		}
		if tok.typ == tokEOF {
			return nil, fmt.Errorf("%w: unexpected EOF inside array at position %d", ErrCorruptObject, savedPos)
		}

		// Backtrack and parse the next object in the array
		p.lexer.SetPos(savedPos)
		obj, err := p.ParseObject()
		if err != nil {
			return nil, err
		}
		arr = append(arr, obj)
	}
	return arr, nil
}

func (p *Parser) parseDictionary() (Dictionary, error) {
	dict := make(Dictionary)
	for {
		savedPos := p.lexer.Pos()
		tok, err := p.lexer.NextToken()
		if err != nil {
			return nil, fmt.Errorf("%w: failed inside dictionary: %v", ErrCorruptObject, err)
		}
		if tok.typ == tokRightDict {
			break
		}
		if tok.typ == tokEOF {
			return nil, fmt.Errorf("%w: unexpected EOF inside dictionary at position %d", ErrCorruptObject, savedPos)
		}

		if tok.typ != tokName {
			return nil, fmt.Errorf("%w: expected name key in dictionary, got '%s' at position %d", ErrCorruptObject, tok.typ, savedPos)
		}
		key := Name(tok.val)

		val, err := p.ParseObject()
		if err != nil {
			return nil, err
		}
		dict[key] = val
	}
	return dict, nil
}

func (p *Parser) parseStreamData(dict Dictionary) (*Stream, error) {
	var length int64 = -1
	if lenObj, ok := dict[Name("Length")]; ok {
		if lenInt, ok := AsInt(lenObj); ok {
			length = lenInt
		}
	}

	startPos := p.lexer.Pos()
	var data []byte

	// If Length is valid and does not exceed file size, try reading directly
	if length >= 0 && startPos+int(length) <= len(p.lexer.input) {
		data = p.lexer.Slice(startPos, startPos+int(length))
		p.lexer.SetPos(startPos + int(length))

		// Check if next token is indeed endstream
		tok, err := p.lexer.NextToken()
		if err == nil && tok.typ == tokEndStream {
			return &Stream{Dict: dict, Data: data}, nil
		}
		// If not, backtrack and fallback to scanning
		p.lexer.SetPos(startPos)
	}

	// Fallback scanning method to find "endstream"
	input := p.lexer.input
	relativeEndstreamIdx := bytes.Index(input[startPos:], []byte("endstream"))
	endstreamIdx := -1
	if relativeEndstreamIdx >= 0 {
		endstreamIdx = startPos + relativeEndstreamIdx
	}

	if endstreamIdx == -1 {
		return nil, fmt.Errorf("%w: could not find 'endstream' for stream starting at position %d", ErrCorruptObject, startPos)
	}

	// Trim newline characters directly preceding "endstream"
	dataEndIdx := endstreamIdx
	if dataEndIdx > startPos && input[dataEndIdx-1] == '\n' {
		dataEndIdx--
		if dataEndIdx > startPos && input[dataEndIdx-1] == '\r' {
			dataEndIdx--
		}
	}

	data = p.lexer.Slice(startPos, dataEndIdx)
	p.lexer.SetPos(endstreamIdx + 9)

	return &Stream{Dict: dict, Data: data}, nil
}
