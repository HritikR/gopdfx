package pdf

import (
	"fmt"
	"strings"
)

type tokenType int

const (
	tokError tokenType = iota
	tokEOF
	tokNull
	tokBool
	tokInteger
	tokReal
	tokName
	tokString
	tokHexString
	tokLeftBracket  // [
	tokRightBracket // ]
	tokLeftDict     // <<
	tokRightDict    // >>
	tokObj          // obj
	tokEndObj       // endobj
	tokStream       // stream
	tokEndStream    // endstream
	tokRef          // R
	tokXref         // xref
	tokTrailer      // trailer
	tokStartXref    // startxref
	tokKeyword      // fallback for other keywords/operators (e.g. Tj, TJ, BT, ET, cm, etc.)
)

func (t tokenType) String() string {
	switch t {
	case tokError:
		return "Error"
	case tokEOF:
		return "EOF"
	case tokNull:
		return "Null"
	case tokBool:
		return "Bool"
	case tokInteger:
		return "Integer"
	case tokReal:
		return "Real"
	case tokName:
		return "Name"
	case tokString:
		return "String"
	case tokHexString:
		return "HexString"
	case tokLeftBracket:
		return "["
	case tokRightBracket:
		return "]"
	case tokLeftDict:
		return "<<"
	case tokRightDict:
		return ">>"
	case tokObj:
		return "obj"
	case tokEndObj:
		return "endobj"
	case tokStream:
		return "stream"
	case tokEndStream:
		return "endstream"
	case tokRef:
		return "R"
	case tokXref:
		return "xref"
	case tokTrailer:
		return "trailer"
	case tokStartXref:
		return "startxref"
	case tokKeyword:
		return "Keyword"
	default:
		return fmt.Sprintf("Unknown(%d)", t)
	}
}

type token struct {
	typ tokenType
	val string
	raw string
}

type Lexer struct {
	input []byte
	pos   int
}

func NewLexer(input []byte) *Lexer {
	return &Lexer{
		input: input,
		pos:   0,
	}
}

func (l *Lexer) Pos() int {
	return l.pos
}

func (l *Lexer) SetPos(pos int) {
	if pos < 0 {
		l.pos = 0
	} else if pos > len(l.input) {
		l.pos = len(l.input)
	} else {
		l.pos = pos
	}
}

func (l *Lexer) SkipBytes(n int) {
	l.SetPos(l.pos + n)
}

func (l *Lexer) Slice(start, end int) []byte {
	if start < 0 {
		start = 0
	}
	if end > len(l.input) {
		end = len(l.input)
	}
	if start > end {
		return nil
	}
	return l.input[start:end]
}

// NextToken scans the next token from the input.
func (l *Lexer) NextToken() (token, error) {
	for {
		l.skipWhitespace()
		if l.pos >= len(l.input) {
			return token{typ: tokEOF}, nil
		}

		// Comment check
		if l.input[l.pos] == '%' {
			l.skipComment()
			continue
		}

		break
	}

	b := l.input[l.pos]

	// Delimiters
	switch b {
	case '[':
		l.pos++
		return token{typ: tokLeftBracket, val: "[", raw: "["}, nil
	case ']':
		l.pos++
		return token{typ: tokRightBracket, val: "]", raw: "]"}, nil
	case '(':
		val, err := l.parseLiteralString()
		if err != nil {
			return token{typ: tokError, val: err.Error()}, err
		}
		return token{typ: tokString, val: val, raw: val}, nil
	case '/':
		val, err := l.parseName()
		if err != nil {
			return token{typ: tokError, val: err.Error()}, err
		}
		return token{typ: tokName, val: val, raw: "/" + val}, nil
	case '<':
		// Check for << or hex string <...>
		if l.pos+1 < len(l.input) && l.input[l.pos+1] == '<' {
			l.pos += 2
			return token{typ: tokLeftDict, val: "<<", raw: "<<"}, nil
		}
		val, err := l.parseHexString()
		if err != nil {
			return token{typ: tokError, val: err.Error()}, err
		}
		return token{typ: tokHexString, val: val, raw: val}, nil
	case '>':
		// Check for >> or unmatched >
		if l.pos+1 < len(l.input) && l.input[l.pos+1] == '>' {
			l.pos += 2
			return token{typ: tokRightDict, val: ">>", raw: ">>"}, nil
		}
		l.pos++
		return token{typ: tokError, val: "unmatched '>'"}, fmt.Errorf("unmatched '>' at position %d", l.pos-1)
	}

	// Regular characters, keywords, numbers
	return l.parseRegularToken()
}

func (l *Lexer) skipWhitespace() {
	for l.pos < len(l.input) && isWhitespace(l.input[l.pos]) {
		l.pos++
	}
}

func (l *Lexer) skipComment() {
	l.pos++ // consume '%'
	for l.pos < len(l.input) {
		b := l.input[l.pos]
		if b == '\r' || b == '\n' {
			break
		}
		l.pos++
	}
}

func (l *Lexer) parseLiteralString() (string, error) {
	l.pos++ // consume '('
	var sb strings.Builder
	parenDepth := 1
	for l.pos < len(l.input) {
		b := l.input[l.pos]
		if b == '(' {
			parenDepth++
			sb.WriteByte(b)
			l.pos++
		} else if b == ')' {
			parenDepth--
			if parenDepth == 0 {
				l.pos++ // consume closing ')'
				return sb.String(), nil
			}
			sb.WriteByte(b)
			l.pos++
		} else if b == '\\' {
			l.pos++ // consume '\\'
			if l.pos >= len(l.input) {
				return "", fmt.Errorf("unexpected EOF after backslash in string at position %d", l.pos)
			}
			b2 := l.input[l.pos]
			switch b2 {
			case 'n':
				sb.WriteByte('\n')
				l.pos++
			case 'r':
				sb.WriteByte('\r')
				l.pos++
			case 't':
				sb.WriteByte('\t')
				l.pos++
			case 'b':
				sb.WriteByte('\b')
				l.pos++
			case 'f':
				sb.WriteByte('\f')
				l.pos++
			case '(', ')', '\\':
				sb.WriteByte(b2)
				l.pos++
			case '\r', '\n':
				// Line continuation
				if b2 == '\r' && l.pos+1 < len(l.input) && l.input[l.pos+1] == '\n' {
					l.pos++
				}
				l.pos++
			default:
				// Check for octal sequence: up to 3 octal digits
				if b2 >= '0' && b2 <= '7' {
					octVal := int(b2 - '0')
					l.pos++
					for i := 0; i < 2; i++ {
						if l.pos < len(l.input) {
							nextB := l.input[l.pos]
							if nextB >= '0' && nextB <= '7' {
								octVal = octVal*8 + int(nextB-'0')
								l.pos++
							} else {
								break
							}
						}
					}
					sb.WriteByte(byte(octVal))
				} else {
					// Treat as literal character
					sb.WriteByte(b2)
					l.pos++
				}
			}
		} else {
			sb.WriteByte(b)
			l.pos++
		}
	}
	return "", fmt.Errorf("unterminated literal string at position %d", l.pos)
}

func (l *Lexer) parseHexString() (string, error) {
	l.pos++ // consume '<'
	var hexDigits []byte
	for l.pos < len(l.input) {
		b := l.input[l.pos]
		if b == '>' {
			l.pos++ // consume '>'
			break
		}
		if isWhitespace(b) {
			l.pos++
			continue
		}
		if isHexDigit(b) {
			hexDigits = append(hexDigits, b)
			l.pos++
		} else {
			// Lenient handling of invalid characters in hex strings
			l.pos++
		}
	}

	var decoded []byte
	for i := 0; i < len(hexDigits); i += 2 {
		var h1, h2 byte
		h1 = hexDigits[i]
		if i+1 < len(hexDigits) {
			h2 = hexDigits[i+1]
		} else {
			h2 = '0'
		}
		val := hexVal(h1)*16 + hexVal(h2)
		decoded = append(decoded, byte(val))
	}
	return string(decoded), nil
}

func (l *Lexer) parseName() (string, error) {
	l.pos++ // consume '/'
	var sb strings.Builder
	for l.pos < len(l.input) {
		b := l.input[l.pos]
		if isWhitespace(b) || isDelimiter(b) {
			break
		}
		if b == '#' {
			if l.pos+2 < len(l.input) {
				h1 := l.input[l.pos+1]
				h2 := l.input[l.pos+2]
				if isHexDigit(h1) && isHexDigit(h2) {
					val := hexVal(h1)*16 + hexVal(h2)
					sb.WriteByte(byte(val))
					l.pos += 3
					continue
				}
			}
		}
		sb.WriteByte(b)
		l.pos++
	}
	return sb.String(), nil
}

func (l *Lexer) parseRegularToken() (token, error) {
	start := l.pos
	for l.pos < len(l.input) {
		b := l.input[l.pos]
		if isWhitespace(b) || isDelimiter(b) {
			break
		}
		l.pos++
	}
	val := string(l.input[start:l.pos])
	if val == "" {
		return token{typ: tokError, val: "empty token"}, fmt.Errorf("empty token at position %d", start)
	}

	switch val {
	case "null":
		return token{typ: tokNull, val: val, raw: val}, nil
	case "true":
		return token{typ: tokBool, val: "true", raw: val}, nil
	case "false":
		return token{typ: tokBool, val: "false", raw: val}, nil
	case "obj":
		return token{typ: tokObj, val: val, raw: val}, nil
	case "endobj":
		return token{typ: tokEndObj, val: val, raw: val}, nil
	case "stream":
		// Handle stream keyword specifically by advancing past the trailing newline character(s).
		// PDF specification states: stream is followed by either CARRIAGE RETURN (0x0d) and LINE FEED (0x0a)
		// or just LINE FEED (0x0a).
		tok := token{typ: tokStream, val: val, raw: val}
		if l.pos < len(l.input) {
			if l.input[l.pos] == '\r' {
				l.pos++
				if l.pos < len(l.input) && l.input[l.pos] == '\n' {
					l.pos++
				}
			} else if l.input[l.pos] == '\n' {
				l.pos++
			}
		}
		return tok, nil
	case "endstream":
		return token{typ: tokEndStream, val: val, raw: val}, nil
	case "R":
		return token{typ: tokRef, val: val, raw: val}, nil
	case "xref":
		return token{typ: tokXref, val: val, raw: val}, nil
	case "trailer":
		return token{typ: tokTrailer, val: val, raw: val}, nil
	case "startxref":
		return token{typ: tokStartXref, val: val, raw: val}, nil
	}

	if isIntegerPattern(val) {
		return token{typ: tokInteger, val: val, raw: val}, nil
	}

	if isRealPattern(val) {
		return token{typ: tokReal, val: val, raw: val}, nil
	}

	return token{typ: tokKeyword, val: val, raw: val}, nil
}

func isWhitespace(b byte) bool {
	return b == 0x00 || b == 0x09 || b == 0x0A || b == 0x0C || b == 0x0D || b == 0x20
}

func isDelimiter(b byte) bool {
	return b == '(' || b == ')' || b == '<' || b == '>' || b == '[' || b == ']' || b == '{' || b == '}' || b == '/' || b == '%'
}

func isHexDigit(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}

func hexVal(b byte) int {
	if b >= '0' && b <= '9' {
		return int(b - '0')
	}
	if b >= 'a' && b <= 'f' {
		return int(b - 'a' + 10)
	}
	if b >= 'A' && b <= 'F' {
		return int(b - 'A' + 10)
	}
	return 0
}

func isIntegerPattern(s string) bool {
	if len(s) == 0 {
		return false
	}
	if s[0] == '+' || s[0] == '-' {
		s = s[1:]
	}
	if len(s) == 0 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func isRealPattern(s string) bool {
	if len(s) == 0 {
		return false
	}
	if s[0] == '+' || s[0] == '-' {
		s = s[1:]
	}
	if len(s) == 0 {
		return false
	}

	dotCount := 0
	digitCount := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			dotCount++
			if dotCount > 1 {
				return false
			}
		} else if s[i] >= '0' && s[i] <= '9' {
			digitCount++
		} else {
			return false
		}
	}
	return digitCount > 0
}
