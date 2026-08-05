package pdf

import (
	"fmt"
	"math"
	"sort"
	"strconv"
)

// BoundingBox represents an axis-aligned rectangle in PDF user-space
// coordinates. X and Y identify the lower-left corner.
type BoundingBox struct {
	X      float64
	Y      float64
	Width  float64
	Height float64
}

func (b BoundingBox) Left() float64   { return b.X }
func (b BoundingBox) Right() float64  { return b.X + b.Width }
func (b BoundingBox) Bottom() float64 { return b.Y }
func (b BoundingBox) Top() float64    { return b.Y + b.Height }

// LineElement is a visible stroked line in page user-space coordinates.
type LineElement struct {
	X1        float64
	Y1        float64
	X2        float64
	Y2        float64
	LineWidth float64
}

// RectangleElement represents a rectangle introduced with the PDF "re"
// operator. Bounds are axis-aligned page-space bounds after transformation.
type RectangleElement struct {
	Bounds    BoundingBox
	LineWidth float64
	Stroked   bool
	Filled    bool
}

// GraphicElements contains page-level vector graphics useful for detecting
// ruled tables.
type GraphicElements struct {
	Lines      []LineElement
	Rectangles []RectangleElement
}

// BoundedTextElement associates an existing text element with the ruled table
// cell that surrounds its X/Y position. CellBounds is nil if no closed cell
// can be inferred from the detected lines.
type BoundedTextElement struct {
	TextElement
	CellBounds *BoundingBox
}

type graphicsPoint struct {
	x float64
	y float64
}

type graphicsMatrix struct {
	a float64
	b float64
	c float64
	d float64
	e float64
	f float64
}

func identityGraphicsMatrix() graphicsMatrix {
	return graphicsMatrix{a: 1, d: 1}
}

// multiply returns m*n. With points treated as column vectors, a PDF cm
// operation concatenates its matrix on the right of the current CTM.
func (m graphicsMatrix) multiply(n graphicsMatrix) graphicsMatrix {
	return graphicsMatrix{
		a: m.a*n.a + m.c*n.b,
		b: m.b*n.a + m.d*n.b,
		c: m.a*n.c + m.c*n.d,
		d: m.b*n.c + m.d*n.d,
		e: m.a*n.e + m.c*n.f + m.e,
		f: m.b*n.e + m.d*n.f + m.f,
	}
}

func (m graphicsMatrix) transform(x, y float64) graphicsPoint {
	return graphicsPoint{
		x: m.a*x + m.c*y + m.e,
		y: m.b*x + m.d*y + m.f,
	}
}

// approximateScale converts a line width from the current graphics space into
// page user space. Non-uniform scaling has no single exact scalar width, so
// this uses the average magnitude of the transformed basis vectors.
func (m graphicsMatrix) approximateScale() float64 {
	sx := math.Hypot(m.a, m.b)
	sy := math.Hypot(m.c, m.d)

	switch {
	case sx == 0 && sy == 0:
		return 1
	case sx == 0:
		return sy
	case sy == 0:
		return sx
	default:
		return (sx + sy) / 2
	}
}

type graphicsState struct {
	ctm       graphicsMatrix
	lineWidth float64
}

type graphicsSegment struct {
	start graphicsPoint
	end   graphicsPoint
}

type pendingRectangle struct {
	corners [4]graphicsPoint
}

type graphicsPath struct {
	segments     []graphicsSegment
	rectangles   []pendingRectangle
	current      graphicsPoint
	subpathStart graphicsPoint
	hasCurrent   bool
	hasSubpath   bool
}

func (p *graphicsPath) reset() {
	p.segments = nil
	p.rectangles = nil
	p.hasCurrent = false
	p.hasSubpath = false
}

func (p *graphicsPath) moveTo(pt graphicsPoint) {
	p.current = pt
	p.subpathStart = pt
	p.hasCurrent = true
	p.hasSubpath = true
}

func (p *graphicsPath) lineTo(pt graphicsPoint) {
	if !p.hasCurrent {
		p.moveTo(pt)
		return
	}

	p.segments = append(p.segments, graphicsSegment{
		start: p.current,
		end:   pt,
	})
	p.current = pt
}

func (p *graphicsPath) close() {
	if !p.hasCurrent || !p.hasSubpath {
		return
	}

	if !pointsNear(p.current, p.subpathStart, 1e-9) {
		p.segments = append(p.segments, graphicsSegment{
			start: p.current,
			end:   p.subpathStart,
		})
	}
	p.current = p.subpathStart
}

func pointsNear(a, b graphicsPoint, tolerance float64) bool {
	return math.Abs(a.x-b.x) <= tolerance && math.Abs(a.y-b.y) <= tolerance
}

// GraphicElements extracts stroked paths and rectangles from the page content.
// It handles the operators commonly used for ruled tables: q, Q, cm, w, m, l,
// h, re, S, s, B, B*, b, b*, f, F, f*, and n.
func (p *Page) GraphicElements() (*GraphicElements, error) {
	content, err := p.contentBytes()
	if err != nil {
		return nil, err
	}
	if len(content) == 0 {
		return &GraphicElements{}, nil
	}

	return p.doc.extractGraphicElements(content)
}

// LineElements is a convenience wrapper around GraphicElements.
func (p *Page) LineElements() ([]LineElement, error) {
	graphics, err := p.GraphicElements()
	if err != nil {
		return nil, err
	}
	return graphics.Lines, nil
}

func (d *Document) extractGraphicElements(content []byte) (*GraphicElements, error) {
	result := &GraphicElements{}
	state := graphicsState{
		ctm:       identityGraphicsMatrix(),
		lineWidth: 1,
	}
	var stateStack []graphicsState
	var operands []Object
	var path graphicsPath

	lexer := NewLexer(content)
	for {
		tok, err := lexer.NextToken()
		if err != nil {
			return nil, fmt.Errorf("lex graphics content: %w", err)
		}
		if tok.typ == tokEOF {
			break
		}

		switch tok.typ {
		case tokInteger:
			value, err := strconv.ParseInt(tok.val, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("parse integer %q: %w", tok.val, err)
			}
			operands = append(operands, Integer(value))

		case tokReal:
			value, err := strconv.ParseFloat(tok.val, 64)
			if err != nil {
				return nil, fmt.Errorf("parse real %q: %w", tok.val, err)
			}
			operands = append(operands, Real(value))

		case tokBool:
			operands = append(operands, Boolean(tok.val == "true"))

		case tokName:
			operands = append(operands, Name(tok.val))

		case tokString:
			operands = append(operands, StringObj{Value: tok.val, Hex: false})

		case tokHexString:
			operands = append(operands, StringObj{Value: tok.val, Hex: true})

		case tokLeftBracket:
			array, err := parseContentArray(lexer)
			if err != nil {
				return nil, fmt.Errorf("parse graphics array: %w", err)
			}
			operands = append(operands, array)

		case tokLeftDict:
			dict, err := parseContentDict(lexer)
			if err != nil {
				return nil, fmt.Errorf("parse graphics dictionary: %w", err)
			}
			operands = append(operands, dict)

		case tokKeyword:
			op := tok.val

			switch op {
			case "q":
				stateStack = append(stateStack, state)

			case "Q":
				if len(stateStack) > 0 {
					state = stateStack[len(stateStack)-1]
					stateStack = stateStack[:len(stateStack)-1]
				}

			case "cm":
				values, ok := lastFloats(operands, 6)
				if ok {
					matrix := graphicsMatrix{
						a: values[0],
						b: values[1],
						c: values[2],
						d: values[3],
						e: values[4],
						f: values[5],
					}
					state.ctm = state.ctm.multiply(matrix)
				}

			case "w":
				values, ok := lastFloats(operands, 1)
				if ok {
					state.lineWidth = values[0]
				}

			case "m":
				values, ok := lastFloats(operands, 2)
				if ok {
					path.moveTo(state.ctm.transform(values[0], values[1]))
				}

			case "l":
				values, ok := lastFloats(operands, 2)
				if ok {
					path.lineTo(state.ctm.transform(values[0], values[1]))
				}

			case "h":
				path.close()

			case "re":
				values, ok := lastFloats(operands, 4)
				if ok {
					x := values[0]
					y := values[1]
					width := values[2]
					height := values[3]

					corners := [4]graphicsPoint{
						state.ctm.transform(x, y),
						state.ctm.transform(x+width, y),
						state.ctm.transform(x+width, y+height),
						state.ctm.transform(x, y+height),
					}

					path.rectangles = append(path.rectangles, pendingRectangle{corners: corners})
					path.moveTo(corners[0])
					path.lineTo(corners[1])
					path.lineTo(corners[2])
					path.lineTo(corners[3])
					path.close()
				}

			case "S":
				emitGraphicsPath(result, &path, state, true, false)

			case "s":
				path.close()
				emitGraphicsPath(result, &path, state, true, false)

			case "B", "B*":
				emitGraphicsPath(result, &path, state, true, true)

			case "b", "b*":
				path.close()
				emitGraphicsPath(result, &path, state, true, true)

			case "f", "F", "f*":
				emitGraphicsPath(result, &path, state, false, true)

			case "n":
				path.reset()
			}

			// PDF operators consume their operands. Clearing here also prevents
			// text/color operands from leaking into later graphics operators.
			operands = nil
		}
	}

	return result, nil
}

func lastFloats(operands []Object, count int) ([]float64, bool) {
	if count <= 0 || len(operands) < count {
		return nil, false
	}

	values := make([]float64, count)
	start := len(operands) - count
	for i := 0; i < count; i++ {
		value, ok := AsFloat(operands[start+i])
		if !ok {
			return nil, false
		}
		values[i] = value
	}
	return values, true
}

func emitGraphicsPath(
	result *GraphicElements,
	path *graphicsPath,
	state graphicsState,
	stroked bool,
	filled bool,
) {
	pageLineWidth := math.Abs(state.lineWidth) * state.ctm.approximateScale()

	if stroked {
		for _, segment := range path.segments {
			result.Lines = append(result.Lines, LineElement{
				X1:        segment.start.x,
				Y1:        segment.start.y,
				X2:        segment.end.x,
				Y2:        segment.end.y,
				LineWidth: pageLineWidth,
			})
		}
	}

	for _, rectangle := range path.rectangles {
		bounds := boundsForPoints(rectangle.corners[:])
		result.Rectangles = append(result.Rectangles, RectangleElement{
			Bounds:    bounds,
			LineWidth: pageLineWidth,
			Stroked:   stroked,
			Filled:    filled,
		})

		// Many PDF generators draw table rules as very thin filled rectangles
		// rather than stroked paths. Convert those shapes into center lines.
		if filled && !stroked {
			const thinRuleLimit = 3.0
			switch {
			case bounds.Height > 0 && bounds.Height <= thinRuleLimit && bounds.Width > bounds.Height:
				y := bounds.Y + bounds.Height/2
				result.Lines = append(result.Lines, LineElement{
					X1:        bounds.Left(),
					Y1:        y,
					X2:        bounds.Right(),
					Y2:        y,
					LineWidth: bounds.Height,
				})

			case bounds.Width > 0 && bounds.Width <= thinRuleLimit && bounds.Height > bounds.Width:
				x := bounds.X + bounds.Width/2
				result.Lines = append(result.Lines, LineElement{
					X1:        x,
					Y1:        bounds.Bottom(),
					X2:        x,
					Y2:        bounds.Top(),
					LineWidth: bounds.Width,
				})
			}
		}
	}

	path.reset()
}

func boundsForPoints(points []graphicsPoint) BoundingBox {
	if len(points) == 0 {
		return BoundingBox{}
	}

	minX := points[0].x
	maxX := points[0].x
	minY := points[0].y
	maxY := points[0].y

	for _, point := range points[1:] {
		minX = math.Min(minX, point.x)
		maxX = math.Max(maxX, point.x)
		minY = math.Min(minY, point.y)
		maxY = math.Max(maxY, point.y)
	}

	return BoundingBox{
		X:      minX,
		Y:      minY,
		Width:  maxX - minX,
		Height: maxY - minY,
	}
}

func isHorizontalLine(line LineElement, tolerance float64) bool {
	return math.Abs(line.Y1-line.Y2) <= tolerance
}

func isVerticalLine(line LineElement, tolerance float64) bool {
	return math.Abs(line.X1-line.X2) <= tolerance
}

func normalizedLine(line LineElement, tolerance float64) LineElement {
	if isHorizontalLine(line, tolerance) {
		y := (line.Y1 + line.Y2) / 2
		line.Y1 = y
		line.Y2 = y
		if line.X1 > line.X2 {
			line.X1, line.X2 = line.X2, line.X1
		}
	}

	if isVerticalLine(line, tolerance) {
		x := (line.X1 + line.X2) / 2
		line.X1 = x
		line.X2 = x
		if line.Y1 > line.Y2 {
			line.Y1, line.Y2 = line.Y2, line.Y1
		}
	}

	return line
}

// MergeLineElements normalizes and merges horizontal/vertical line fragments.
// tolerance controls axis alignment and coordinate grouping; maxGap controls
// how far apart adjacent fragments may be and still count as one border.
func MergeLineElements(lines []LineElement, tolerance, maxGap float64) []LineElement {
	if tolerance <= 0 {
		tolerance = 1
	}
	if maxGap < 0 {
		maxGap = 0
	}

	var horizontal []LineElement
	var vertical []LineElement
	var other []LineElement

	for _, raw := range lines {
		line := normalizedLine(raw, tolerance)
		switch {
		case isHorizontalLine(line, tolerance):
			horizontal = append(horizontal, line)
		case isVerticalLine(line, tolerance):
			vertical = append(vertical, line)
		default:
			other = append(other, line)
		}
	}

	sort.Slice(horizontal, func(i, j int) bool {
		if math.Abs(horizontal[i].Y1-horizontal[j].Y1) > tolerance {
			return horizontal[i].Y1 < horizontal[j].Y1
		}
		return horizontal[i].X1 < horizontal[j].X1
	})

	sort.Slice(vertical, func(i, j int) bool {
		if math.Abs(vertical[i].X1-vertical[j].X1) > tolerance {
			return vertical[i].X1 < vertical[j].X1
		}
		return vertical[i].Y1 < vertical[j].Y1
	})

	mergedHorizontal := mergeHorizontalLines(horizontal, tolerance, maxGap)
	mergedVertical := mergeVerticalLines(vertical, tolerance, maxGap)

	result := make([]LineElement, 0, len(mergedHorizontal)+len(mergedVertical)+len(other))
	result = append(result, mergedHorizontal...)
	result = append(result, mergedVertical...)
	result = append(result, other...)
	return result
}

func mergeHorizontalLines(lines []LineElement, tolerance, maxGap float64) []LineElement {
	var merged []LineElement

	for _, line := range lines {
		if len(merged) == 0 {
			merged = append(merged, line)
			continue
		}

		last := &merged[len(merged)-1]
		sameAxis := math.Abs(last.Y1-line.Y1) <= tolerance
		touches := line.X1 <= last.X2+maxGap+tolerance

		if sameAxis && touches {
			last.X1 = math.Min(last.X1, line.X1)
			last.X2 = math.Max(last.X2, line.X2)
			last.Y1 = (last.Y1 + line.Y1) / 2
			last.Y2 = last.Y1
			last.LineWidth = math.Max(last.LineWidth, line.LineWidth)
			continue
		}

		merged = append(merged, line)
	}

	return merged
}

func mergeVerticalLines(lines []LineElement, tolerance, maxGap float64) []LineElement {
	var merged []LineElement

	for _, line := range lines {
		if len(merged) == 0 {
			merged = append(merged, line)
			continue
		}

		last := &merged[len(merged)-1]
		sameAxis := math.Abs(last.X1-line.X1) <= tolerance
		touches := line.Y1 <= last.Y2+maxGap+tolerance

		if sameAxis && touches {
			last.Y1 = math.Min(last.Y1, line.Y1)
			last.Y2 = math.Max(last.Y2, line.Y2)
			last.X1 = (last.X1 + line.X1) / 2
			last.X2 = last.X1
			last.LineWidth = math.Max(last.LineWidth, line.LineWidth)
			continue
		}

		merged = append(merged, line)
	}

	return merged
}

// FindCellBounds returns the nearest closed ruled-table cell surrounding x/y.
// A candidate border must span the lookup point on its perpendicular axis.
func FindCellBounds(
	x float64,
	y float64,
	lines []LineElement,
	tolerance float64,
) (BoundingBox, bool) {
	if tolerance <= 0 {
		tolerance = 1
	}

	var left, right, bottom, top float64
	var hasLeft, hasRight, hasBottom, hasTop bool

	for _, raw := range lines {
		line := normalizedLine(raw, tolerance)

		switch {
		case isVerticalLine(line, tolerance):
			lineX := (line.X1 + line.X2) / 2
			minY := math.Min(line.Y1, line.Y2)
			maxY := math.Max(line.Y1, line.Y2)

			if y < minY-tolerance || y > maxY+tolerance {
				continue
			}

			if lineX < x-tolerance && (!hasLeft || lineX > left) {
				left = lineX
				hasLeft = true
			}
			if lineX > x+tolerance && (!hasRight || lineX < right) {
				right = lineX
				hasRight = true
			}

		case isHorizontalLine(line, tolerance):
			lineY := (line.Y1 + line.Y2) / 2
			minX := math.Min(line.X1, line.X2)
			maxX := math.Max(line.X1, line.X2)

			if x < minX-tolerance || x > maxX+tolerance {
				continue
			}

			if lineY < y-tolerance && (!hasBottom || lineY > bottom) {
				bottom = lineY
				hasBottom = true
			}
			if lineY > y+tolerance && (!hasTop || lineY < top) {
				top = lineY
				hasTop = true
			}
		}
	}

	if !hasLeft || !hasRight || !hasBottom || !hasTop {
		return BoundingBox{}, false
	}
	if right-left <= tolerance || top-bottom <= tolerance {
		return BoundingBox{}, false
	}

	// Ensure the four selected borders really overlap the proposed corners.
	if !hasHorizontalBorder(lines, bottom, left, right, tolerance) ||
		!hasHorizontalBorder(lines, top, left, right, tolerance) ||
		!hasVerticalBorder(lines, left, bottom, top, tolerance) ||
		!hasVerticalBorder(lines, right, bottom, top, tolerance) {
		return BoundingBox{}, false
	}

	return BoundingBox{
		X:      left,
		Y:      bottom,
		Width:  right - left,
		Height: top - bottom,
	}, true
}

func hasHorizontalBorder(lines []LineElement, y, left, right, tolerance float64) bool {
	for _, raw := range lines {
		line := normalizedLine(raw, tolerance)
		if !isHorizontalLine(line, tolerance) || math.Abs(line.Y1-y) > tolerance {
			continue
		}
		if line.X1 <= left+tolerance && line.X2 >= right-tolerance {
			return true
		}
	}
	return false
}

func hasVerticalBorder(lines []LineElement, x, bottom, top, tolerance float64) bool {
	for _, raw := range lines {
		line := normalizedLine(raw, tolerance)
		if !isVerticalLine(line, tolerance) || math.Abs(line.X1-x) > tolerance {
			continue
		}
		if line.Y1 <= bottom+tolerance && line.Y2 >= top-tolerance {
			return true
		}
	}
	return false
}

// TextElementsWithCellBounds returns the existing text elements together with
// table-cell bounds inferred from the page's vector lines.
func (p *Page) TextElementsWithCellBounds() ([]BoundedTextElement, error) {
	texts, err := p.TextElements()
	if err != nil {
		return nil, err
	}

	graphics, err := p.GraphicElements()
	if err != nil {
		return nil, err
	}

	// These defaults work well for ordinary PDF table rules measured in points.
	// Adjust them for unusually fragmented or imprecise documents.
	const (
		axisTolerance = 1.0
		mergeGap      = 1.5
	)

	lines := MergeLineElements(graphics.Lines, axisTolerance, mergeGap)
	result := make([]BoundedTextElement, len(texts))

	for i, text := range texts {
		result[i].TextElement = text
		if bounds, ok := FindCellBounds(text.X, text.Y, lines, axisTolerance); ok {
			cellBounds := bounds
			result[i].CellBounds = &cellBounds
		}
	}

	return result, nil
}
