package pdf

import (
	"fmt"
	"strconv"
	"strings"
)

// Object is the interface representing all PDF objects.
type Object interface {
	String() string
}

// Null represents the PDF null object.
type Null struct{}

func (n Null) String() string {
	return "null"
}

// Boolean represents the PDF boolean object (true or false).
type Boolean bool

func (b Boolean) String() string {
	return strconv.FormatBool(bool(b))
}

// Integer represents the PDF integer numeric object.
type Integer int64

func (i Integer) String() string {
	return strconv.FormatInt(int64(i), 10)
}

// Real represents the PDF real numeric object.
type Real float64

func (r Real) String() string {
	return strconv.FormatFloat(float64(r), 'f', -1, 64)
}

// Name represents the PDF name object (e.g., /Type).
// The value stored is the decoded name without the leading slash.
type Name string

func (n Name) String() string {
	// Escape special characters as per PDF spec if necessary.
	// For debugging/printing, we prepend the slash and escape special bytes.
	var sb strings.Builder
	sb.WriteByte('/')
	for i := 0; i < len(n); i++ {
		b := n[i]
		// Delimiters or non-ascii/whitespace should be escaped using #XX hex representation.
		if b <= 32 || b >= 127 || b == '#' || b == '/' || b == '(' || b == ')' || b == '<' || b == '>' || b == '[' || b == ']' || b == '{' || b == '}' || b == '%' {
			sb.WriteString(fmt.Sprintf("#%02X", b))
		} else {
			sb.WriteByte(b)
		}
	}
	return sb.String()
}

// StringObj represents the PDF string object.
// It can be a literal string (e.g., (hello)) or hexadecimal string (e.g., <4E6F>).
type StringObj struct {
	Value string
	Hex   bool
}

func (s StringObj) String() string {
	if s.Hex {
		var sb strings.Builder
		sb.WriteByte('<')
		for i := 0; i < len(s.Value); i++ {
			sb.WriteString(fmt.Sprintf("%02X", s.Value[i]))
		}
		sb.WriteByte('>')
		return sb.String()
	}
	// For literal string, escape parentheses and backslashes
	var sb strings.Builder
	sb.WriteByte('(')
	for i := 0; i < len(s.Value); i++ {
		b := s.Value[i]
		switch b {
		case '(':
			sb.WriteString(`\(`)
		case ')':
			sb.WriteString(`\)`)
		case '\\':
			sb.WriteString(`\\`)
		case '\r':
			sb.WriteString(`\r`)
		case '\n':
			sb.WriteString(`\n`)
		case '\t':
			sb.WriteString(`\t`)
		case '\b':
			sb.WriteString(`\b`)
		case '\f':
			sb.WriteString(`\f`)
		default:
			sb.WriteByte(b)
		}
	}
	sb.WriteByte(')')
	return sb.String()
}

// Array represents the PDF array object (e.g., [1 2 (three)]).
type Array []Object

func (a Array) String() string {
	var parts []string
	for _, obj := range a {
		if obj == nil {
			parts = append(parts, "null")
		} else {
			parts = append(parts, obj.String())
		}
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// Dictionary represents the PDF dictionary object (e.g., << /Type /Catalog >>).
type Dictionary map[Name]Object

func (d Dictionary) String() string {
	var parts []string
	for k, v := range d {
		valStr := "null"
		if v != nil {
			valStr = v.String()
		}
		parts = append(parts, k.String()+" "+valStr)
	}
	// We sort or just join. Note: order is not guaranteed, but for simple printing it's fine.
	return "<< " + strings.Join(parts, " ") + " >>"
}

// IndirectRef represents the PDF indirect reference (e.g., 12 0 R).
type IndirectRef struct {
	ObjectNumber     int64
	GenerationNumber int64
}

func (r IndirectRef) String() string {
	return fmt.Sprintf("%d %d R", r.ObjectNumber, r.GenerationNumber)
}

// IndirectObj represents the PDF indirect object definition (e.g., 12 0 obj ... endobj).
type IndirectObj struct {
	ObjectNumber     int64
	GenerationNumber int64
	Value            Object
}

func (io IndirectObj) String() string {
	valStr := "null"
	if io.Value != nil {
		valStr = io.Value.String()
	}
	return fmt.Sprintf("%d %d obj\n%s\nendobj", io.ObjectNumber, io.GenerationNumber, valStr)
}

// Stream represents the PDF stream object (e.g., << /Length 12 >> stream\n...\nendstream).
type Stream struct {
	Dict Dictionary
	Data []byte
}

func (s Stream) String() string {
	// A stream is represented by its dictionary and "stream ... endstream"
	return fmt.Sprintf("%s stream\n[bytes: %d]\nendstream", s.Dict.String(), len(s.Data))
}

// Helpers to extract types safely

func AsBool(obj Object) (bool, bool) {
	if b, ok := obj.(Boolean); ok {
		return bool(b), true
	}
	return false, false
}

func AsInt(obj Object) (int64, bool) {
	if i, ok := obj.(Integer); ok {
		return int64(i), true
	}
	return 0, false
}

func AsFloat(obj Object) (float64, bool) {
	switch v := obj.(type) {
	case Integer:
		return float64(v), true
	case Real:
		return float64(v), true
	}
	return 0.0, false
}

func AsName(obj Object) (string, bool) {
	if n, ok := obj.(Name); ok {
		return string(n), true
	}
	return "", false
}

func AsString(obj Object) (string, bool) {
	if s, ok := obj.(StringObj); ok {
		return s.Value, true
	}
	return "", false
}

func AsArray(obj Object) (Array, bool) {
	if a, ok := obj.(Array); ok {
		return a, true
	}
	return nil, false
}

func AsDict(obj Object) (Dictionary, bool) {
	if d, ok := obj.(Dictionary); ok {
		return d, true
	}
	return nil, false
}

func AsRef(obj Object) (IndirectRef, bool) {
	if r, ok := obj.(IndirectRef); ok {
		return r, true
	}
	return IndirectRef{}, false
}

func AsIndirectObj(obj Object) (*IndirectObj, bool) {
	if io, ok := obj.(*IndirectObj); ok {
		return io, true
	}
	if io, ok := obj.(IndirectObj); ok {
		return &io, true
	}
	return nil, false
}

func AsStream(obj Object) (*Stream, bool) {
	if s, ok := obj.(*Stream); ok {
		return s, true
	}
	if s, ok := obj.(Stream); ok {
		return &s, true
	}
	return nil, false
}
