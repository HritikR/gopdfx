package pdf

import (
	"fmt"
	"strconv"
)

type XrefType int

const (
	XrefEntryFree XrefType = iota
	XrefEntryNormal
	XrefEntryCompressed
)

func (t XrefType) String() string {
	switch t {
	case XrefEntryFree:
		return "Free"
	case XrefEntryNormal:
		return "Normal"
	case XrefEntryCompressed:
		return "Compressed"
	default:
		return "Unknown"
	}
}

type XrefEntry struct {
	Type             XrefType
	Offset           int64 // For Normal: file offset. For Compressed: Object number of ObjStm.
	GenerationNumber int   // For Normal: generation number. For Compressed: index in ObjStm.
}

// ParseXrefTable parses a classic PDF xref table starting at the current lexer position.
func ParseXrefTable(p *Parser) (map[int64]XrefEntry, error) {
	entries := make(map[int64]XrefEntry)

	tok, err := p.lexer.NextToken()
	if err != nil {
		return nil, fmt.Errorf("%w: failed to read xref token: %v", ErrCorruptObject, err)
	}
	if tok.typ != tokXref {
		return nil, fmt.Errorf("%w: expected 'xref' keyword, got '%s'", ErrCorruptObject, tok.val)
	}

	for {
		savedPos := p.lexer.Pos()
		tok, err := p.lexer.NextToken()
		if err != nil {
			return nil, fmt.Errorf("%w: failed to read xref subsection start: %v", ErrCorruptObject, err)
		}

		// Reached the trailer dictionary
		if tok.typ == tokTrailer {
			p.lexer.SetPos(savedPos) // Backtrack so trailer parser can consume "trailer"
			break
		}

		if tok.typ != tokInteger {
			return nil, fmt.Errorf("%w: expected integer for subsection start object number, got '%s'", ErrCorruptObject, tok.val)
		}
		startObj, err := strconv.ParseInt(tok.val, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid start object number '%s': %v", ErrCorruptObject, tok.val, err)
		}

		tokCount, err := p.lexer.NextToken()
		if err != nil {
			return nil, fmt.Errorf("%w: failed to read xref subsection count: %v", ErrCorruptObject, err)
		}
		if tokCount.typ != tokInteger {
			return nil, fmt.Errorf("%w: expected integer for subsection count, got '%s'", ErrCorruptObject, tokCount.val)
		}
		count, err := strconv.ParseInt(tokCount.val, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid count '%s': %v", ErrCorruptObject, tokCount.val, err)
		}

		for i := int64(0); i < count; i++ {
			tokOffset, err := p.lexer.NextToken()
			if err != nil {
				return nil, fmt.Errorf("%w: failed to read xref entry offset at index %d: %v", ErrCorruptObject, i, err)
			}
			if tokOffset.typ != tokInteger {
				return nil, fmt.Errorf("%w: expected integer for entry offset, got '%s'", ErrCorruptObject, tokOffset.val)
			}
			offset, err := strconv.ParseInt(tokOffset.val, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("%w: invalid offset '%s': %v", ErrCorruptObject, tokOffset.val, err)
			}

			tokGen, err := p.lexer.NextToken()
			if err != nil {
				return nil, fmt.Errorf("%w: failed to read xref entry generation: %v", ErrCorruptObject, err)
			}
			if tokGen.typ != tokInteger {
				return nil, fmt.Errorf("%w: expected integer for entry generation, got '%s'", ErrCorruptObject, tokGen.val)
			}
			gen, err := strconv.ParseInt(tokGen.val, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("%w: invalid generation '%s': %v", ErrCorruptObject, tokGen.val, err)
			}

			tokFlag, err := p.lexer.NextToken()
			if err != nil {
				return nil, fmt.Errorf("%w: failed to read xref entry flag: %v", ErrCorruptObject, err)
			}
			if tokFlag.typ != tokKeyword {
				return nil, fmt.Errorf("%w: expected keyword 'n' or 'f' for entry flag, got '%s'", ErrCorruptObject, tokFlag.val)
			}

			var entryType XrefType
			if tokFlag.val == "n" {
				entryType = XrefEntryNormal
			} else if tokFlag.val == "f" {
				entryType = XrefEntryFree
			} else {
				return nil, fmt.Errorf("%w: invalid xref entry flag '%s' (expected 'n' or 'f')", ErrCorruptObject, tokFlag.val)
			}

			objNum := startObj + i
			entries[objNum] = XrefEntry{
				Type:             entryType,
				Offset:           offset,
				GenerationNumber: int(gen),
			}
		}
	}

	return entries, nil
}

// ParseXrefStream parses a binary PDF cross-reference stream object.
func ParseXrefStream(stream *Stream) (map[int64]XrefEntry, error) {
	entries := make(map[int64]XrefEntry)

	// Size is mandatory
	sizeObj, ok := stream.Dict[Name("Size")]
	if !ok {
		return nil, fmt.Errorf("%w: missing Size in xref stream dictionary", ErrCorruptObject)
	}
	size, ok := AsInt(sizeObj)
	if !ok {
		return nil, fmt.Errorf("%w: Size is not an integer in xref stream dictionary", ErrCorruptObject)
	}

	// Index is optional. Defaults to [0, Size]
	var index []int64
	if idxObj, ok := stream.Dict[Name("Index")]; ok {
		if idxArr, ok := AsArray(idxObj); ok {
			for _, val := range idxArr {
				if iVal, ok := AsInt(val); ok {
					index = append(index, iVal)
				}
			}
		}
	}
	if len(index) == 0 {
		index = []int64{0, size}
	}

	// W is mandatory
	wObj, ok := stream.Dict[Name("W")]
	if !ok {
		return nil, fmt.Errorf("%w: missing W in xref stream dictionary", ErrCorruptObject)
	}
	wArr, ok := AsArray(wObj)
	if !ok {
		return nil, fmt.Errorf("%w: W is not an array in xref stream dictionary", ErrCorruptObject)
	}

	var w []int
	for _, val := range wArr {
		if iVal, ok := AsInt(val); ok {
			w = append(w, int(iVal))
		}
	}
	if len(w) < 3 {
		return nil, fmt.Errorf("%w: /W array must contain at least 3 elements, got %d", ErrCorruptObject, len(w))
	}

	entrySize := w[0] + w[1] + w[2]
	if entrySize <= 0 {
		return nil, fmt.Errorf("%w: invalid /W values: sum of entry widths must be > 0", ErrCorruptObject)
	}

	data := stream.Data
	offset := 0

	for s := 0; s < len(index); s += 2 {
		if s+1 >= len(index) {
			break
		}
		startObj := index[s]
		count := index[s+1]

		for i := int64(0); i < count; i++ {
			if offset+entrySize > len(data) {
				break
			}

			// Field 1 (Type of entry, defaults to 1 if width is 0)
			f1 := int64(1)
			if w[0] > 0 {
				f1 = decodeInt(data, offset, w[0])
			}
			f2 := decodeInt(data, offset+w[0], w[1])
			f3 := decodeInt(data, offset+w[0]+w[1], w[2])

			var entryType XrefType
			switch f1 {
			case 0:
				entryType = XrefEntryFree
			case 1:
				entryType = XrefEntryNormal
			case 2:
				entryType = XrefEntryCompressed
			default:
				// Invalid type, treat as free
				entryType = XrefEntryFree
			}

			objNum := startObj + i
			entries[objNum] = XrefEntry{
				Type:             entryType,
				Offset:           f2,
				GenerationNumber: int(f3),
			}

			offset += entrySize
		}
	}

	return entries, nil
}

func decodeInt(data []byte, start int, width int) int64 {
	if width <= 0 {
		return 0
	}
	var val int64
	for i := 0; i < width; i++ {
		val = (val << 8) | int64(data[start+i])
	}
	return val
}
