package pdf

import (
	"bytes"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

type Page struct {
	doc  *Document
	Dict Dictionary
	Ref  IndirectRef
}

type CMap struct {
	bfchar  map[uint32]string
	bfrange []CMapRange
}

type CMapRange struct {
	Start uint32
	End   uint32
	Dst   uint32
	Array []string
}

type Font struct {
	Name     Name // PDF resource name, e.g. F1
	BaseFont Name // Embedded font name, e.g. ABCDEE+Calibri
	Subtype  Name
	CMap     *CMap
}

type TextElement struct {
	X        float64
	Y        float64
	Text     string
	FontName string
	FontSize float64
}

type textElement struct {
	x        float64
	y        float64
	text     string
	idx      int
	font     *Font
	fontSize float64
}

func (c *CMap) Lookup(code uint32) (string, bool) {
	if c == nil {
		return "", false
	}
	if s, ok := c.bfchar[code]; ok {
		return s, true
	}
	for _, r := range c.bfrange {
		if code >= r.Start && code <= r.End {
			offset := code - r.Start
			if len(r.Array) > 0 {
				if offset < uint32(len(r.Array)) {
					return r.Array[offset], true
				}
			} else {
				return string(rune(r.Dst + offset)), true
			}
		}
	}
	return "", false
}

func bytesToUint32(b []byte) uint32 {
	var val uint32
	for _, x := range b {
		val = (val << 8) | uint32(x)
	}
	return val
}

func decodeUTF16BEBytes(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	if len(b) == 1 {
		return string(b)
	}
	if len(b)%2 != 0 {
		return string(b)
	}
	var runes []uint16
	for i := 0; i < len(b); i += 2 {
		runes = append(runes, (uint16(b[i])<<8)|uint16(b[i+1]))
	}
	return string(utf16.Decode(runes))
}

func cleanFontName(name Name) string {
	s := string(name)

	// Embedded subset fonts commonly use a six-character prefix:
	// ABCDEF+Calibri-Bold
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[i+1:]
	}

	return s
}

func ParseCMap(data []byte) (*CMap, error) {
	cmap := &CMap{
		bfchar: make(map[uint32]string),
	}

	l := NewLexer(data)
	for {
		tok, err := l.NextToken()
		if err != nil || tok.typ == tokEOF {
			break
		}
		if tok.typ == tokKeyword && tok.val == "beginbfchar" {
			for {
				t, err := l.NextToken()
				if err != nil || t.typ == tokEOF || (t.typ == tokKeyword && t.val == "endbfchar") {
					break
				}
				if t.typ != tokHexString {
					continue
				}
				srcCode := bytesToUint32([]byte(t.val))

				t2, err := l.NextToken()
				if err != nil || t2.typ == tokEOF {
					break
				}

				if t2.typ == tokHexString {
					cmap.bfchar[srcCode] = decodeUTF16BEBytes([]byte(t2.val))
				} else if t2.typ == tokString {
					cmap.bfchar[srcCode] = t2.val
				}
			}
		} else if tok.typ == tokKeyword && tok.val == "beginbfrange" {
			for {
				t, err := l.NextToken()
				if err != nil || t.typ == tokEOF || (t.typ == tokKeyword && t.val == "endbfrange") {
					break
				}
				if t.typ != tokHexString {
					continue
				}
				startCode := bytesToUint32([]byte(t.val))

				t2, err := l.NextToken()
				if err != nil || t2.typ != tokHexString {
					continue
				}
				endCode := bytesToUint32([]byte(t2.val))

				t3, err := l.NextToken()
				if err != nil {
					continue
				}

				if t3.typ == tokHexString {
					dstCode := bytesToUint32([]byte(t3.val))
					cmap.bfrange = append(cmap.bfrange, CMapRange{
						Start: startCode,
						End:   endCode,
						Dst:   dstCode,
					})
				} else if t3.typ == tokLeftBracket {
					var arr []string
					for {
						item, err := l.NextToken()
						if err != nil || item.typ == tokRightBracket {
							break
						}
						if item.typ == tokHexString {
							arr = append(arr, decodeUTF16BEBytes([]byte(item.val)))
						} else if item.typ == tokString {
							arr = append(arr, item.val)
						}
					}
					cmap.bfrange = append(cmap.bfrange, CMapRange{
						Start: startCode,
						End:   endCode,
						Array: arr,
					})
				}
			}
		}
	}
	return cmap, nil
}

func (f *Font) DecodeString(str string) string {
	var result []rune

	if f.CMap != nil {
		isMultiByte := (f.Subtype == "Type0")
		bytesVal := []byte(str)
		if isMultiByte {
			for i := 0; i < len(bytesVal); i += 2 {
				if i+1 < len(bytesVal) {
					code := (uint32(bytesVal[i]) << 8) | uint32(bytesVal[i+1])
					if unicode, ok := f.CMap.Lookup(code); ok {
						result = append(result, []rune(unicode)...)
					} else {
						result = append(result, rune(code))
					}
				}
			}
		} else {
			for _, b := range bytesVal {
				code := uint32(b)
				if unicode, ok := f.CMap.Lookup(code); ok {
					result = append(result, []rune(unicode)...)
				} else {
					result = append(result, rune(b))
				}
			}
		}
		return string(result)
	}

	for _, b := range []byte(str) {
		result = append(result, rune(b))
	}
	return string(result)
}

func (d *Document) initPageRefs() error {
	if d.pageRefs != nil {
		return nil
	}

	catalog, err := d.Catalog()
	if err != nil {
		return err
	}

	pagesRefObj, ok := catalog[Name("Pages")]
	if !ok {
		return fmt.Errorf("missing /Pages in catalog")
	}

	pagesRef, ok := AsRef(pagesRefObj)
	if !ok {
		return fmt.Errorf("/Pages is not an indirect reference")
	}

	var refs []IndirectRef
	err = d.collectPages(pagesRef, &refs)
	if err != nil {
		return err
	}

	d.pageRefs = refs
	return nil
}

func (d *Document) collectPages(ref IndirectRef, pages *[]IndirectRef) error {
	return d.collectPagesVisited(ref, pages, make(map[IndirectRef]struct{}))
}

func (d *Document) collectPagesVisited(ref IndirectRef, pages *[]IndirectRef, active map[IndirectRef]struct{}) error {
	if _, seen := active[ref]; seen {
		return fmt.Errorf("cycle in page tree at object %d", ref.ObjectNumber)
	}
	active[ref] = struct{}{}
	defer delete(active, ref)

	obj, err := d.Resolve(ref)
	if err != nil {
		return err
	}
	dict, ok := AsDict(obj)
	if !ok {
		return fmt.Errorf("invalid page tree node at object %d", ref.ObjectNumber)
	}

	typeVal, _ := AsName(dict[Name("Type")])
	if typeVal == "Pages" {
		kidsObj, ok := dict[Name("Kids")]
		if !ok {
			return fmt.Errorf("missing Kids in pages tree node %d", ref.ObjectNumber)
		}
		kids, ok := AsArray(kidsObj)
		if !ok {
			return fmt.Errorf("invalid Kids array in pages tree node %d", ref.ObjectNumber)
		}
		for _, kidRefObj := range kids {
			kidRef, ok := AsRef(kidRefObj)
			if !ok {
				return fmt.Errorf("invalid Kid reference in pages tree node %d", ref.ObjectNumber)
			}
			err = d.collectPagesVisited(kidRef, pages, active)
			if err != nil {
				return err
			}
		}
	} else if typeVal == "Page" {
		*pages = append(*pages, ref)
	} else {
		return fmt.Errorf("unknown page tree node type /%s in object %d", typeVal, ref.ObjectNumber)
	}
	return nil
}

func (d *Document) resolveObject(obj Object) (Object, error) {
	if ref, ok := AsRef(obj); ok {
		return d.Resolve(ref)
	}
	return obj, nil
}

func (d *Document) getPageResource(pageDict Dictionary, resourceType Name, resourceName Name) (Object, error) {
	curr := pageDict
	visited := make(map[IndirectRef]struct{})

	for {
		if resObj, ok := curr[Name("Resources")]; ok {
			resolved, err := d.resolveObject(resObj)
			if err != nil {
				return nil, fmt.Errorf("resolve /Resources: %w", err)
			}
			if resDict, ok := AsDict(resolved); ok {
				if typeObj, ok := resDict[resourceType]; ok {
					resolvedType, err := d.resolveObject(typeObj)
					if err != nil {
						return nil, fmt.Errorf("resolve /%s resources: %w", resourceType, err)
					}
					if typeDict, ok := AsDict(resolvedType); ok {
						if val, ok := typeDict[resourceName]; ok {
							return d.resolveObject(val)
						}
					}
				}
			}
		}

		parentRefObj, ok := curr[Name("Parent")]
		if !ok {
			break
		}
		parentRef, ok := AsRef(parentRefObj)
		if !ok {
			return nil, fmt.Errorf("invalid /Parent reference")
		}
		if _, seen := visited[parentRef]; seen {
			return nil, fmt.Errorf("cycle in page parent chain at object %d", parentRef.ObjectNumber)
		}
		visited[parentRef] = struct{}{}

		parentObj, err := d.Resolve(parentRef)
		if err != nil {
			return nil, fmt.Errorf("resolve page parent %d: %w", parentRef.ObjectNumber, err)
		}
		var okDict bool
		curr, okDict = AsDict(parentObj)
		if !okDict {
			return nil, fmt.Errorf("page parent %d is not a dictionary", parentRef.ObjectNumber)
		}
	}
	return nil, fmt.Errorf("resource /%s /%s not found", resourceType, resourceName)
}

func (d *Document) loadFont(fontName Name, fontDict Dictionary) (*Font, error) {
	f := &Font{Name: fontName}

	if subtypeObj, ok := fontDict[Name("Subtype")]; ok {
		if subtype, ok := AsName(subtypeObj); ok {
			f.Subtype = Name(subtype)
		}
	}

	if baseFontObj, ok := fontDict[Name("BaseFont")]; ok {
		if baseFont, ok := AsName(baseFontObj); ok {
			f.BaseFont = Name(baseFont)
		}
	}

	toUnicodeObj, ok := fontDict[Name("ToUnicode")]
	if !ok {
		return f, nil
	}

	resolved, err := d.resolveObject(toUnicodeObj)
	if err != nil {
		return nil, fmt.Errorf("resolve ToUnicode for font /%s: %w", fontName, err)
	}
	stream, ok := AsStream(resolved)
	if !ok {
		return nil, fmt.Errorf("ToUnicode for font /%s is not a stream", fontName)
	}
	decodedData, err := stream.Decode()
	if err != nil {
		return nil, fmt.Errorf("decode ToUnicode for font /%s: %w", fontName, err)
	}
	f.CMap, err = ParseCMap(decodedData)
	if err != nil {
		return nil, fmt.Errorf("parse ToUnicode for font /%s: %w", fontName, err)
	}
	return f, nil
}

func (d *Document) readStreamData(ref IndirectRef) ([]byte, error) {
	obj, err := d.Resolve(ref)
	if err != nil {
		return nil, err
	}
	stream, ok := AsStream(obj)
	if !ok {
		return nil, fmt.Errorf("expected stream, got %T", obj)
	}
	return stream.Decode()
}

func (d *Document) extractTextElements(content []byte, pageDict Dictionary) ([]TextElement, error) {
	resolvedFonts := make(map[Name]*Font)
	var currentFont *Font
	var currentFontSize float64
	var stack []Object
	var elements []textElement
	elementCount := 0

	// Layout and position coordinates tracking
	var tx, ty float64
	leading := 12.0

	l := NewLexer(content)
	for {
		tok, err := l.NextToken()
		if err != nil || tok.typ == tokEOF {
			break
		}

		switch tok.typ {
		case tokInteger:
			val, _ := strconv.ParseInt(tok.val, 10, 64)
			stack = append(stack, Integer(val))
		case tokReal:
			val, _ := strconv.ParseFloat(tok.val, 64)
			stack = append(stack, Real(val))
		case tokBool:
			stack = append(stack, Boolean(tok.val == "true"))
		case tokName:
			stack = append(stack, Name(tok.val))
		case tokString:
			stack = append(stack, StringObj{Value: tok.val, Hex: false})
		case tokHexString:
			stack = append(stack, StringObj{Value: tok.val, Hex: true})
		case tokLeftBracket:
			arr, err := parseContentArray(l)
			if err != nil {
				return nil, err
			}
			stack = append(stack, arr)
		case tokLeftDict:
			dict, err := parseContentDict(l)
			if err != nil {
				return nil, err
			}
			stack = append(stack, dict)
		case tokKeyword:
			op := tok.val
			switch op {
			case "BT":
				tx = 0.0
				ty = 0.0
				stack = nil
			case "ET":
				stack = nil
			case "Tf":
				if len(stack) >= 2 {
					if fontNameStr, ok := AsName(stack[len(stack)-2]); ok {
						fontName := Name(fontNameStr)

						f, found := resolvedFonts[fontName]
						if !found {
							// A content stream can still be useful when its font
							// resource is unavailable. Keep currentFont nil so the
							// text operators use their raw-byte fallback instead of
							// failing the entire extraction.
							if pageDict != nil {
								fontRes, err := d.getPageResource(
									pageDict,
									Name("Font"),
									fontName,
								)
								if err == nil {
									fontDict, ok := AsDict(fontRes)
									if !ok {
										return nil, fmt.Errorf(
											"font resource /%s is %T, expected dictionary",
											fontName,
											fontRes,
										)
									}

									f, err = d.loadFont(fontName, fontDict)
									if err != nil {
										return nil, err
									}

									resolvedFonts[fontName] = f
								}
							}
						}

						currentFont = f
					}

					if size, ok := AsFloat(stack[len(stack)-1]); ok {
						currentFontSize = size
					}
				}

				stack = nil
			case "Td":
				if len(stack) >= 2 {
					xVal, _ := AsFloat(stack[len(stack)-2])
					yVal, _ := AsFloat(stack[len(stack)-1])
					tx += xVal
					ty += yVal
				}
				stack = nil
			case "TD":
				if len(stack) >= 2 {
					xVal, _ := AsFloat(stack[len(stack)-2])
					yVal, _ := AsFloat(stack[len(stack)-1])
					leading = -yVal
					tx += xVal
					ty += yVal
				}
				stack = nil
			case "Tm":
				if len(stack) >= 6 {
					xVal, _ := AsFloat(stack[len(stack)-2])
					yVal, _ := AsFloat(stack[len(stack)-1])
					tx = xVal
					ty = yVal
				}
				stack = nil
			case "T*":
				ty -= leading
				stack = nil
			case "Tj":
				if len(stack) >= 1 {
					if strObj, ok := AsString(stack[len(stack)-1]); ok {
						text := strObj
						if currentFont != nil {
							text = currentFont.DecodeString(strObj)
						}
						elements = append(elements, textElement{
							x:        tx,
							y:        ty,
							text:     text,
							idx:      elementCount,
							font:     currentFont,
							fontSize: currentFontSize,
						})
						elementCount++
					}
				}
				stack = nil
			case "TJ":
				if len(stack) >= 1 {
					if arr, ok := AsArray(stack[len(stack)-1]); ok {
						var sb strings.Builder
						for _, item := range arr {
							if strObj, ok := AsString(item); ok {
								text := strObj
								if currentFont != nil {
									text = currentFont.DecodeString(strObj)
								}
								sb.WriteString(text)
							} else if num, ok := AsFloat(item); ok {
								if num <= -100 {
									sb.WriteByte(' ')
								}
							}
						}
						elements = append(elements, textElement{
							x:        tx,
							y:        ty,
							text:     sb.String(),
							idx:      elementCount,
							font:     currentFont,
							fontSize: currentFontSize,
						})
						elementCount++
					}
				}
				stack = nil
			case "'":
				ty -= leading
				if len(stack) >= 1 {
					if strObj, ok := AsString(stack[len(stack)-1]); ok {
						text := strObj
						if currentFont != nil {
							text = currentFont.DecodeString(strObj)
						}
						elements = append(elements, textElement{
							x:        tx,
							y:        ty,
							text:     text,
							idx:      elementCount,
							font:     currentFont,
							fontSize: currentFontSize,
						})
						elementCount++
					}
				}
				stack = nil
			case "\"":
				ty -= leading
				if len(stack) >= 1 {
					if strObj, ok := AsString(stack[len(stack)-1]); ok {
						text := strObj
						if currentFont != nil {
							text = currentFont.DecodeString(strObj)
						}
						elements = append(elements, textElement{
							x:        tx,
							y:        ty,
							text:     text,
							idx:      elementCount,
							font:     currentFont,
							fontSize: currentFontSize,
						})
						elementCount++
					}
				}
				stack = nil
			default:
				stack = nil
			}
		}
	}

	// Layout Reconstruction: Group elements into rows by their Y coordinate
	type row struct {
		y        float64
		elements []textElement
	}
	var rows []row

	for _, el := range elements {
		found := false
		for i := range rows {
			if math.Abs(rows[i].y-el.y) <= 2.0 {
				rows[i].elements = append(rows[i].elements, el)
				found = true
				break
			}
		}
		if !found {
			rows = append(rows, row{
				y:        el.y,
				elements: []textElement{el},
			})
		}
	}

	// Sort rows top-to-bottom (Y descending)
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].y > rows[j].y
	})

	var sortedElements []TextElement
	for _, r := range rows {
		// Sort elements within each row left-to-right (X ascending)
		sort.Slice(r.elements, func(i, j int) bool {
			if r.elements[i].x == r.elements[j].x {
				return r.elements[i].idx < r.elements[j].idx
			}
			return r.elements[i].x < r.elements[j].x
		})

		for _, el := range r.elements {
			result := TextElement{
				X:        el.x,
				Y:        el.y,
				Text:     el.text,
				FontSize: el.fontSize,
			}

			if el.font != nil {
				result.FontName = cleanFontName(el.font.BaseFont)

				// Some unusual fonts may not contain /BaseFont.
				if result.FontName == "" {
					result.FontName = string(el.font.Name)
				}
			}

			sortedElements = append(sortedElements, result)
		}
	}

	return sortedElements, nil
}

func (d *Document) parseContentStream(content []byte, pageDict Dictionary) (string, error) {
	elements, err := d.extractTextElements(content, pageDict)
	if err != nil {
		return "", err
	}

	var result bytes.Buffer
	var lastY float64
	var hasLastY bool

	for _, el := range elements {
		if !hasLastY {
			lastY = el.Y
			hasLastY = true
		} else if math.Abs(el.Y-lastY) > 2.0 {
			result.WriteByte('\n')
			lastY = el.Y
		} else {
			b := result.Bytes()
			// Insert space separator between columns if not already spaced
			if len(b) > 0 && b[len(b)-1] != ' ' && b[len(b)-1] != '\n' {
				result.WriteByte(' ')
			}
		}
		result.WriteString(el.Text)
	}

	// Normalize trailing newlines
	res := result.String()
	if len(res) > 0 && res[len(res)-1] != '\n' {
		res += "\n"
	}
	return res, nil
}

func parseContentArray(l *Lexer) (Array, error) {
	var arr Array
	for {
		tok, err := l.NextToken()
		if err != nil || tok.typ == tokEOF || tok.typ == tokRightBracket {
			break
		}
		switch tok.typ {
		case tokInteger:
			val, _ := strconv.ParseInt(tok.val, 10, 64)
			arr = append(arr, Integer(val))
		case tokReal:
			val, _ := strconv.ParseFloat(tok.val, 64)
			arr = append(arr, Real(val))
		case tokBool:
			arr = append(arr, Boolean(tok.val == "true"))
		case tokName:
			arr = append(arr, Name(tok.val))
		case tokString:
			arr = append(arr, StringObj{Value: tok.val, Hex: false})
		case tokHexString:
			arr = append(arr, StringObj{Value: tok.val, Hex: true})
		case tokLeftBracket:
			sub, err := parseContentArray(l)
			if err != nil {
				return nil, err
			}
			arr = append(arr, sub)
		case tokLeftDict:
			sub, err := parseContentDict(l)
			if err != nil {
				return nil, err
			}
			arr = append(arr, sub)
		}
	}
	return arr, nil
}

func parseContentDict(l *Lexer) (Dictionary, error) {
	dict := make(Dictionary)
	for {
		tok, err := l.NextToken()
		if err != nil || tok.typ == tokEOF || tok.typ == tokRightDict {
			break
		}
		if tok.typ != tokName {
			continue
		}
		key := Name(tok.val)

		valTok, err := l.NextToken()
		if err != nil || valTok.typ == tokEOF {
			break
		}

		var val Object
		switch valTok.typ {
		case tokInteger:
			v, _ := strconv.ParseInt(valTok.val, 10, 64)
			val = Integer(v)
		case tokReal:
			v, _ := strconv.ParseFloat(valTok.val, 64)
			val = Real(v)
		case tokBool:
			val = Boolean(valTok.val == "true")
		case tokName:
			val = Name(valTok.val)
		case tokString:
			val = StringObj{Value: valTok.val, Hex: false}
		case tokHexString:
			val = StringObj{Value: valTok.val, Hex: true}
		case tokLeftBracket:
			sub, err := parseContentArray(l)
			if err == nil {
				val = sub
			}
		case tokLeftDict:
			sub, err := parseContentDict(l)
			if err == nil {
				val = sub
			}
		}
		if val != nil {
			dict[key] = val
		}
	}
	return dict, nil
}

func (d *Document) NumPages() (int, error) {
	err := d.initPageRefs()
	if err != nil {
		return 0, err
	}
	return len(d.pageRefs), nil
}

func (d *Document) Page(index int) (*Page, error) {
	err := d.initPageRefs()
	if err != nil {
		return nil, err
	}
	if index < 0 || index >= len(d.pageRefs) {
		return nil, fmt.Errorf("page index out of range: %d", index)
	}

	ref := d.pageRefs[index]
	pageObj, err := d.Resolve(ref)
	if err != nil {
		return nil, err
	}

	pageDict, ok := AsDict(pageObj)
	if !ok {
		return nil, fmt.Errorf("expected page object to be Dictionary, got %T", pageObj)
	}

	return &Page{
		doc:  d,
		Dict: pageDict,
		Ref:  ref,
	}, nil
}

func (d *Document) Pages() ([]*Page, error) {
	n, err := d.NumPages()
	if err != nil {
		return nil, err
	}

	pages := make([]*Page, n)
	for i := 0; i < n; i++ {
		p, err := d.Page(i)
		if err != nil {
			return nil, err
		}
		pages[i] = p
	}
	return pages, nil
}

func (d *Document) Text() (string, error) {
	pages, err := d.Pages()
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	for i, p := range pages {
		txt, err := p.Text()
		if err != nil {
			return "", err
		}
		buf.WriteString(txt)
		if i < len(pages)-1 {
			buf.WriteByte('\n')
		}
	}
	return buf.String(), nil
}

func (p *Page) contentBytes() ([]byte, error) {
	contentsObj, ok := p.Dict[Name("Contents")]
	if !ok {
		return nil, nil
	}

	readOne := func(obj Object) ([]byte, error) {
		resolved, err := p.doc.resolveObject(obj)
		if err != nil {
			return nil, err
		}
		stream, ok := AsStream(resolved)
		if !ok {
			return nil, fmt.Errorf("expected content stream, got %T", resolved)
		}
		return stream.Decode()
	}

	if arr, ok := AsArray(contentsObj); ok {
		var buf bytes.Buffer
		for i, item := range arr {
			data, err := readOne(item)
			if err != nil {
				return nil, fmt.Errorf("decode content stream %d: %w", i, err)
			}
			if buf.Len() > 0 {
				buf.WriteByte(' ')
			}
			buf.Write(data)
		}
		return buf.Bytes(), nil
	}

	return readOne(contentsObj)
}

func (p *Page) Text() (string, error) {
	content, err := p.contentBytes()
	if err != nil {
		return "", err
	}
	if len(content) == 0 {
		return "", nil
	}
	return p.doc.parseContentStream(content, p.Dict)
}

func (p *Page) TextElements() ([]TextElement, error) {
	content, err := p.contentBytes()
	if err != nil {
		return nil, err
	}
	if len(content) == 0 {
		return nil, nil
	}
	return p.doc.extractTextElements(content, p.Dict)
}
