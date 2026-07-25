package pdf

import (
	"fmt"
	"io"
	"os"
	"strconv"
)

type Document struct {
	input       []byte
	lexer       *Lexer
	parser      *Parser
	xrefTable   map[int64]XrefEntry
	trailer     Dictionary
	objectCache map[int64]Object
}

// Open reads a PDF file from a path and returns a parsed Document.
func Open(path string) (*Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return OpenBytes(data)
}

// OpenReader reads a PDF document from an io.Reader.
func OpenReader(r io.Reader) (*Document, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return OpenBytes(data)
}

// OpenBytes initializes a parsed Document from a byte slice.
func OpenBytes(data []byte) (*Document, error) {
	l := NewLexer(data)
	p := NewParser(l)

	doc := &Document{
		input:       data,
		lexer:       l,
		parser:      p,
		xrefTable:   make(map[int64]XrefEntry),
		trailer:     make(Dictionary),
		objectCache: make(map[int64]Object),
	}

	// 1. Locate startxref offset
	offset, err := FindStartXrefOffset(data)
	if err != nil {
		return nil, err
	}

	// 2. Load xref table(s) and trailer(s) recursively
	err = doc.loadXrefAndTrailer(offset)
	if err != nil {
		return nil, err
	}

	return doc, nil
}

func (d *Document) loadXrefAndTrailer(offset int64) error {
	subLexer := NewLexer(d.input)
	subLexer.SetPos(int(offset))
	p := NewParser(subLexer)

	savedPos := subLexer.Pos()
	tok, err := p.lexer.NextToken()
	if err != nil {
		return fmt.Errorf("failed to read token at xref offset %d: %v", offset, err)
	}

	var currentTrailer Dictionary
	var entries map[int64]XrefEntry

	if tok.typ == tokXref {
		// Classic xref table
		p.lexer.SetPos(savedPos)
		entries, err = ParseXrefTable(p)
		if err != nil {
			return err
		}

		currentTrailer, err = ParseTrailerDict(p)
		if err != nil {
			return err
		}
	} else if tok.typ == tokInteger {
		// Xref Stream (PDF 1.5+)
		p.lexer.SetPos(savedPos)
		obj, err := p.ParseObject()
		if err != nil {
			return fmt.Errorf("failed to parse xref stream object: %v", err)
		}

		iobj, ok := AsIndirectObj(obj)
		if !ok {
			return fmt.Errorf("expected indirect object at xref stream offset, got %T", obj)
		}

		stream, ok := AsStream(iobj.Value)
		if !ok {
			return fmt.Errorf("expected stream value for xref stream object, got %T", iobj.Value)
		}

		typeVal, _ := AsName(stream.Dict[Name("Type")])
		if typeVal != "XRef" {
			return fmt.Errorf("expected /Type /XRef for xref stream, got /%s", typeVal)
		}

		decodedData, err := stream.Decode()
		if err != nil {
			return fmt.Errorf("failed to decode xref stream: %v", err)
		}

		decodedStream := &Stream{
			Dict: stream.Dict,
			Data: decodedData,
		}

		entries, err = ParseXrefStream(decodedStream)
		if err != nil {
			return err
		}

		currentTrailer = stream.Dict
	} else {
		return fmt.Errorf("unexpected token type '%s' at xref offset %d", tok.typ, offset)
	}

	// Merge entries (newer entries parsed first, so do not overwrite)
	for objNum, entry := range entries {
		if _, exists := d.xrefTable[objNum]; !exists {
			d.xrefTable[objNum] = entry
		}
	}

	// Merge trailer keys (newer values parsed first, so do not overwrite)
	for k, v := range currentTrailer {
		if _, exists := d.trailer[k]; !exists {
			d.trailer[k] = v
		}
	}

	// Handle incremental updates by recursively parsing Prev offset
	if prevObj, ok := currentTrailer[Name("Prev")]; ok {
		if prevOffset, ok := AsInt(prevObj); ok && prevOffset > 0 {
			return d.loadXrefAndTrailer(prevOffset)
		}
	}

	return nil
}

// Resolve resolves an indirect reference to its underlying Object.
func (d *Document) Resolve(ref IndirectRef) (Object, error) {
	if obj, ok := d.objectCache[ref.ObjectNumber]; ok {
		return obj, nil
	}

	entry, ok := d.xrefTable[ref.ObjectNumber]
	if !ok {
		// PDF specification: An indirect reference to an undefined object
		// shall be treated as a reference to the null object.
		return Null{}, nil
	}

	switch entry.Type {
	case XrefEntryFree:
		return Null{}, nil

	case XrefEntryNormal:
		obj, err := d.parser.ParseAt(entry.Offset)
		if err != nil {
			return nil, err
		}

		iobj, ok := AsIndirectObj(obj)
		if !ok {
			return nil, fmt.Errorf("expected indirect object at offset %d, got %T", entry.Offset, obj)
		}
		if iobj.ObjectNumber != ref.ObjectNumber {
			return nil, fmt.Errorf("object number mismatch: expected %d, got %d", ref.ObjectNumber, iobj.ObjectNumber)
		}

		d.objectCache[ref.ObjectNumber] = iobj.Value
		return iobj.Value, nil

	case XrefEntryCompressed:
		// Resolve the object stream containing this object
		stmObj, err := d.Resolve(IndirectRef{ObjectNumber: entry.Offset})
		if err != nil {
			return nil, fmt.Errorf("failed to resolve object stream %d: %v", entry.Offset, err)
		}

		stream, ok := AsStream(stmObj)
		if !ok {
			return nil, fmt.Errorf("expected stream for object stream %d, got %T", entry.Offset, stmObj)
		}

		decodedData, err := stream.Decode()
		if err != nil {
			return nil, fmt.Errorf("failed to decode object stream %d: %v", entry.Offset, err)
		}

		decodedStream := &Stream{
			Dict: stream.Dict,
			Data: decodedData,
		}

		// Parse all objects inside the object stream and cache them
		objs, err := d.parseObjectStream(decodedStream)
		if err != nil {
			return nil, fmt.Errorf("failed to parse object stream %d: %v", entry.Offset, err)
		}

		for objNum, objVal := range objs {
			d.objectCache[int64(objNum)] = objVal
		}

		target, ok := d.objectCache[ref.ObjectNumber]
		if !ok {
			return nil, fmt.Errorf("object %d not found in object stream %d", ref.ObjectNumber, entry.Offset)
		}
		return target, nil
	}

	return Null{}, nil
}

func (d *Document) parseObjectStream(stream *Stream) (map[int]Object, error) {
	nVal, ok := AsInt(stream.Dict[Name("N")])
	if !ok {
		return nil, fmt.Errorf("missing or invalid /N in object stream")
	}
	firstVal, ok := AsInt(stream.Dict[Name("First")])
	if !ok {
		return nil, fmt.Errorf("missing or invalid /First in object stream")
	}

	lexer := NewLexer(stream.Data)
	p := NewParser(lexer)

	type ObjStreamEntry struct {
		objNum int64
		offset int64
	}
	entries := make([]ObjStreamEntry, nVal)
	for i := int64(0); i < nVal; i++ {
		tokNum, err := p.lexer.NextToken()
		if err != nil {
			return nil, err
		}
		objNum, _ := strconv.ParseInt(tokNum.val, 10, 64)

		tokOff, err := p.lexer.NextToken()
		if err != nil {
			return nil, err
		}
		off, _ := strconv.ParseInt(tokOff.val, 10, 64)

		entries[i] = ObjStreamEntry{objNum: objNum, offset: off}
	}

	results := make(map[int]Object)
	for _, entry := range entries {
		absOffset := firstVal + entry.offset
		p.lexer.SetPos(int(absOffset))

		obj, err := p.ParseObject()
		if err != nil {
			return nil, fmt.Errorf("failed to parse object %d in object stream: %v", entry.objNum, err)
		}
		results[int(entry.objNum)] = obj
	}

	return results, nil
}

// Catalog returns the document Catalog dictionary (Root entry).
func (d *Document) Catalog() (Dictionary, error) {
	rootObj, ok := d.trailer[Name("Root")]
	if !ok {
		return nil, fmt.Errorf("missing /Root in trailer")
	}

	ref, ok := AsRef(rootObj)
	if !ok {
		return nil, fmt.Errorf("/Root is not an indirect reference")
	}

	catObj, err := d.Resolve(ref)
	if err != nil {
		return nil, err
	}

	catDict, ok := AsDict(catObj)
	if !ok {
		return nil, fmt.Errorf("/Root resolves to %T, expected Dictionary", catObj)
	}

	return catDict, nil
}
