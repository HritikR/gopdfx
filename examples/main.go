package main

import (
	"flag"
	"fmt"
	"html"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/HritikR/gopdfx/pdf"
)

const geometryTolerance = 1.0

type textFragment struct {
	X        float64
	Y        float64
	Text     string
	FontName string
	FontSize float64
}

type tableCell struct {
	Bounds    pdf.BoundingBox
	Fragments []textFragment
}

type tableRow struct {
	Top    float64
	Bottom float64
	Cells  []tableCell
}

func main() {
	password := flag.String(
		"password",
		"",
		"Password to decrypt the PDF (if encrypted)",
	)
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		fmt.Println("Usage: go run examples/main.go [-password <password>] <path-to-pdf>")
		os.Exit(1)
	}

	if err := os.MkdirAll("output", 0o755); err != nil {
		fmt.Printf("Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	pdfPath := args[0]
	fmt.Printf("Opening PDF: %s...\n", pdfPath)

	doc, err := pdf.Open(pdfPath)
	if err != nil {
		fmt.Printf("Error opening PDF: %v\n", err)
		os.Exit(1)
	}

	if doc.Encryption.IsActive() {
		fmt.Println("Document is encrypted. Attempting to unlock...")

		if err := doc.Unlock(*password); err != nil {
			fmt.Printf(
				"Failed to unlock document (incorrect or missing password): %v\n",
				err,
			)
			os.Exit(1)
		}

		fmt.Println("Successfully decrypted and unlocked document!")
	}

	numPages, err := doc.NumPages()
	if err != nil {
		fmt.Printf("Error reading page count: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Total Pages: %d\n\n", numPages)

	for pageIndex := 0; pageIndex < numPages; pageIndex++ {
		if err := exportPage(doc, pageIndex); err != nil {
			fmt.Printf(
				"Error exporting page %d: %v\n",
				pageIndex+1,
				err,
			)
			continue
		}

		fmt.Printf(
			"Successfully exported page %d to output/page_%d.txt and output/page_%d.html\n",
			pageIndex+1,
			pageIndex+1,
			pageIndex+1,
		)
	}
}

func exportPage(doc *pdf.Document, pageIndex int) error {
	page, err := doc.Page(pageIndex)
	if err != nil {
		return fmt.Errorf("load page: %w", err)
	}

	textElements, err := page.TextElements()
	if err != nil {
		return fmt.Errorf(
			"extract text elements: %w",
			err,
		)
	}

	elements := make([]pdf.BoundedTextElement, len(textElements))
	for i, element := range textElements {
		elements[i].TextElement = element
	}

	// Table detection is best-effort. Preserve extracted text when a page has
	// no ruled table or its graphics contain operators this example cannot use.
	lines, lineErr := page.LineElements()
	if lineErr != nil {
		fmt.Printf("Warning: page %d table detection unavailable: %v\n", pageIndex+1, lineErr)
	} else {
		lines = pdf.MergeLineElements(lines, geometryTolerance, 1.5)
		for i := range elements {
			if bounds, ok := pdf.FindCellBounds(elements[i].X, elements[i].Y, lines, geometryTolerance); ok {
				cellBounds := bounds
				elements[i].CellBounds = &cellBounds
			}
		}
	}

	textPath := fmt.Sprintf(
		"output/page_%d.txt",
		pageIndex+1,
	)

	if err := writeDebugText(textPath, elements); err != nil {
		return err
	}

	rows := buildTableRows(
		elements,
		geometryTolerance,
	)

	columnBoundaries := collectColumnBoundaries(
		rows,
		geometryTolerance,
	)

	htmlPath := fmt.Sprintf(
		"output/page_%d.html",
		pageIndex+1,
	)

	if err := writeHTML(
		htmlPath,
		pageIndex+1,
		rows,
		columnBoundaries,
		elements,
	); err != nil {
		return err
	}

	return nil
}

func writeDebugText(
	path string,
	elements []pdf.BoundedTextElement,
) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}

	closed := false

	defer func() {
		if !closed {
			_ = file.Close()
		}
	}()

	for _, element := range elements {
		if element.CellBounds == nil {
			_, err = fmt.Fprintf(
				file,
				"text=%q font=%s fontsize=%f x=%f y=%f cellBounds=none\n",
				element.Text,
				element.FontName,
				element.FontSize,
				element.X,
				element.Y,
			)
		} else {
			_, err = fmt.Fprintf(
				file,
				"text=%q font=%s fontsize=%f x=%f y=%f "+
					"cellX=%f cellY=%f cellWidth=%f cellHeight=%f\n",
				element.Text,
				element.FontName,
				element.FontSize,
				element.X,
				element.Y,
				element.CellBounds.X,
				element.CellBounds.Y,
				element.CellBounds.Width,
				element.CellBounds.Height,
			)
		}

		if err != nil {
			return fmt.Errorf(
				"write %s: %w",
				path,
				err,
			)
		}
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf(
			"close %s: %w",
			path,
			err,
		)
	}

	closed = true
	return nil
}

func buildTableRows(
	elements []pdf.BoundedTextElement,
	tolerance float64,
) []tableRow {
	cells := groupElementsByCell(
		elements,
		tolerance,
	)

	var rows []tableRow

	for _, cell := range cells {
		top := cell.Bounds.Top()
		bottom := cell.Bounds.Bottom()

		rowIndex := findMatchingRow(
			rows,
			top,
			bottom,
			tolerance,
		)

		if rowIndex < 0 {
			rows = append(rows, tableRow{
				Top:    top,
				Bottom: bottom,
			})

			rowIndex = len(rows) - 1
		}

		rows[rowIndex].Cells = append(
			rows[rowIndex].Cells,
			cell,
		)
	}

	// PDF Y coordinates increase from bottom to top.
	sort.SliceStable(rows, func(i, j int) bool {
		if approximatelyEqual(
			rows[i].Top,
			rows[j].Top,
			tolerance,
		) {
			return rows[i].Bottom > rows[j].Bottom
		}

		return rows[i].Top > rows[j].Top
	})

	for i := range rows {
		sort.SliceStable(
			rows[i].Cells,
			func(a, b int) bool {
				return rows[i].Cells[a].Bounds.Left() <
					rows[i].Cells[b].Bounds.Left()
			},
		)
	}

	return rows
}

func groupElementsByCell(
	elements []pdf.BoundedTextElement,
	tolerance float64,
) []tableCell {
	var cells []tableCell

	for _, element := range elements {
		if element.CellBounds == nil {
			// Text without four surrounding borders is not treated
			// as part of the ruled table.
			continue
		}

		cellIndex := findMatchingCell(
			cells,
			*element.CellBounds,
			tolerance,
		)

		if cellIndex < 0 {
			cells = append(cells, tableCell{
				Bounds: *element.CellBounds,
			})

			cellIndex = len(cells) - 1
		}

		cells[cellIndex].Fragments = append(
			cells[cellIndex].Fragments,
			textFragment{
				X:        element.X,
				Y:        element.Y,
				Text:     element.Text,
				FontName: element.FontName,
				FontSize: element.FontSize,
			},
		)
	}

	for i := range cells {
		// Sort cell content top-to-bottom, then left-to-right.
		sort.SliceStable(
			cells[i].Fragments,
			func(a, b int) bool {
				left := cells[i].Fragments[a]
				right := cells[i].Fragments[b]

				if approximatelyEqual(
					left.Y,
					right.Y,
					tolerance,
				) {
					return left.X < right.X
				}

				return left.Y > right.Y
			},
		)
	}

	return cells
}

func findMatchingCell(
	cells []tableCell,
	bounds pdf.BoundingBox,
	tolerance float64,
) int {
	for i := range cells {
		if sameBounds(
			cells[i].Bounds,
			bounds,
			tolerance,
		) {
			return i
		}
	}

	return -1
}

func findMatchingRow(
	rows []tableRow,
	top float64,
	bottom float64,
	tolerance float64,
) int {
	for i := range rows {
		if approximatelyEqual(
			rows[i].Top,
			top,
			tolerance,
		) &&
			approximatelyEqual(
				rows[i].Bottom,
				bottom,
				tolerance,
			) {
			return i
		}
	}

	return -1
}

func collectColumnBoundaries(
	rows []tableRow,
	tolerance float64,
) []float64 {
	var boundaries []float64

	for _, row := range rows {
		for _, cell := range row.Cells {
			boundaries = appendUniqueFloat(
				boundaries,
				cell.Bounds.Left(),
				tolerance,
			)

			boundaries = appendUniqueFloat(
				boundaries,
				cell.Bounds.Right(),
				tolerance,
			)
		}
	}

	sort.Float64s(boundaries)
	return boundaries
}

func appendUniqueFloat(
	values []float64,
	value float64,
	tolerance float64,
) []float64 {
	for _, existing := range values {
		if approximatelyEqual(
			existing,
			value,
			tolerance,
		) {
			return values
		}
	}

	return append(values, value)
}

func boundaryIndex(
	boundaries []float64,
	value float64,
	tolerance float64,
) int {
	for i, boundary := range boundaries {
		if approximatelyEqual(
			boundary,
			value,
			tolerance,
		) {
			return i
		}
	}

	return -1
}

func writeHTML(
	path string,
	pageNumber int,
	rows []tableRow,
	columnBoundaries []float64,
	elements []pdf.BoundedTextElement,
) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf(
			"create %s: %w",
			path,
			err,
		)
	}

	closed := false

	defer func() {
		if !closed {
			_ = file.Close()
		}
	}()

	if _, err := fmt.Fprint(file, `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>
body {
  margin: 20px;
  background: #f7f9fc;
  color: #1a202c;
  font-family: sans-serif;
}

table {
  width: 100%;
  margin-bottom: 30px;
  border-collapse: collapse;
  table-layout: fixed;
  background: #fff;
  box-shadow: 0 4px 6px rgba(0, 0, 0, 0.1);
}

td {
  border: 1px solid #94a3b8;
  padding: 10px;
  vertical-align: top;
  overflow-wrap: anywhere;
  font-family: monospace;
  font-size: 13px;
}

tr:nth-child(even) {
  background: #f8fafc;
}

.empty-cell {
  min-height: 1em;
}

.metadata {
  color: #64748b;
  font-family: sans-serif;
  font-size: 11px;
}

.no-table {
  padding: 12px;
  border: 1px solid #cbd5e1;
  background: #fff;
}

.raw-text {
  padding: 12px;
  border: 1px solid #cbd5e1;
  background: #fff;
  white-space: pre-wrap;
  font-family: monospace;
}
</style>
</head>
<body>
`); err != nil {
		return fmt.Errorf(
			"write %s header: %w",
			path,
			err,
		)
	}

	if _, err := fmt.Fprintf(
		file,
		"<h2>Page %d Table Layout</h2>\n",
		pageNumber,
	); err != nil {
		return fmt.Errorf(
			"write %s title: %w",
			path,
			err,
		)
	}

	if len(rows) == 0 || len(columnBoundaries) < 2 {
		if _, err := fmt.Fprint(
			file,
			"<p class=\"no-table\">"+
				"No line-bounded table cells were detected on this page."+
				"</p>\n",
		); err != nil {
			return fmt.Errorf(
				"write %s empty message: %w",
				path,
				err,
			)
		}

		if err := writeHTMLTextFallback(file, elements); err != nil {
			return fmt.Errorf("write %s text fallback: %w", path, err)
		}
	} else if err := writeHTMLTable(
		file,
		rows,
		columnBoundaries,
		geometryTolerance,
	); err != nil {
		return fmt.Errorf(
			"write %s table: %w",
			path,
			err,
		)
	}

	if _, err := fmt.Fprint(
		file,
		"</body>\n</html>\n",
	); err != nil {
		return fmt.Errorf(
			"write %s footer: %w",
			path,
			err,
		)
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf(
			"close %s: %w",
			path,
			err,
		)
	}

	closed = true
	return nil
}

func writeHTMLTextFallback(file *os.File, elements []pdf.BoundedTextElement) error {
	if _, err := fmt.Fprintln(file, "<pre class=\"raw-text\">"); err != nil {
		return err
	}

	var previousY float64
	hasPreviousY := false
	for _, element := range elements {
		text := strings.TrimSpace(element.Text)
		if text == "" {
			continue
		}

		if hasPreviousY && !approximatelyEqual(previousY, element.Y, geometryTolerance) {
			if _, err := fmt.Fprintln(file); err != nil {
				return err
			}
		} else if hasPreviousY {
			if _, err := fmt.Fprint(file, " "); err != nil {
				return err
			}
		}

		if _, err := fmt.Fprint(file, html.EscapeString(text)); err != nil {
			return err
		}
		previousY = element.Y
		hasPreviousY = true
	}

	_, err := fmt.Fprintln(file, "</pre>")
	return err
}

func writeHTMLTable(
	file *os.File,
	rows []tableRow,
	columnBoundaries []float64,
	tolerance float64,
) error {
	if _, err := fmt.Fprintln(file, "<table>"); err != nil {
		return err
	}

	columnCount := len(columnBoundaries) - 1

	for _, row := range rows {
		if _, err := fmt.Fprintln(file, "  <tr>"); err != nil {
			return err
		}

		cellsByStartColumn := make(map[int]tableCell)

		for _, cell := range row.Cells {
			start := boundaryIndex(
				columnBoundaries,
				cell.Bounds.Left(),
				tolerance,
			)

			if start >= 0 {
				cellsByStartColumn[start] = cell
			}
		}

		for column := 0; column < columnCount; {
			cell, exists := cellsByStartColumn[column]

			if !exists {
				if _, err := fmt.Fprintln(
					file,
					"    <td class=\"empty-cell\"></td>",
				); err != nil {
					return err
				}

				column++
				continue
			}

			end := boundaryIndex(
				columnBoundaries,
				cell.Bounds.Right(),
				tolerance,
			)

			if end <= column {
				end = column + 1
			}

			colspan := end - column

			if err := writeHTMLCell(
				file,
				cell,
				colspan,
			); err != nil {
				return err
			}

			column = end
		}

		if _, err := fmt.Fprintln(file, "  </tr>"); err != nil {
			return err
		}
	}

	_, err := fmt.Fprintln(file, "</table>")
	return err
}

func writeHTMLCell(
	file *os.File,
	cell tableCell,
	colspan int,
) error {
	if colspan < 1 {
		colspan = 1
	}

	text, fontName, fontSize := renderCellContent(
		cell.Fragments,
	)

	_, err := fmt.Fprintf(
		file,
		"    <td colspan=\"%d\" data-x=\"%.2f\" data-y=\"%.2f\" "+
			"data-width=\"%.2f\" data-height=\"%.2f\">"+
			"%s<br><span class=\"metadata\">"+
			"fontName: %s, fontSize: %.2f<br>"+
			"cell: x=%.2f, y=%.2f, width=%.2f, height=%.2f"+
			"</span></td>\n",
		colspan,
		cell.Bounds.X,
		cell.Bounds.Y,
		cell.Bounds.Width,
		cell.Bounds.Height,
		text,
		html.EscapeString(fontName),
		fontSize,
		cell.Bounds.X,
		cell.Bounds.Y,
		cell.Bounds.Width,
		cell.Bounds.Height,
	)

	return err
}

func renderCellContent(
	fragments []textFragment,
) (
	text string,
	fontName string,
	fontSize float64,
) {
	if len(fragments) == 0 {
		return "", "", 0
	}

	var lines []string
	var currentLine []string

	var previousY float64
	hasPreviousY := false

	for _, fragment := range fragments {
		trimmed := strings.TrimSpace(fragment.Text)
		if trimmed == "" {
			continue
		}

		if fontName == "" {
			fontName = fragment.FontName
		}

		if fontSize == 0 {
			fontSize = fragment.FontSize
		}

		if hasPreviousY &&
			!approximatelyEqual(
				previousY,
				fragment.Y,
				geometryTolerance,
			) {
			if len(currentLine) > 0 {
				lines = append(
					lines,
					strings.Join(currentLine, " "),
				)

				currentLine = nil
			}
		}

		currentLine = append(
			currentLine,
			html.EscapeString(trimmed),
		)

		previousY = fragment.Y
		hasPreviousY = true
	}

	if len(currentLine) > 0 {
		lines = append(
			lines,
			strings.Join(currentLine, " "),
		)
	}

	return strings.Join(lines, "<br>"), fontName, fontSize
}

func sameBounds(
	a pdf.BoundingBox,
	b pdf.BoundingBox,
	tolerance float64,
) bool {
	return approximatelyEqual(
		a.X,
		b.X,
		tolerance,
	) &&
		approximatelyEqual(
			a.Y,
			b.Y,
			tolerance,
		) &&
		approximatelyEqual(
			a.Width,
			b.Width,
			tolerance,
		) &&
		approximatelyEqual(
			a.Height,
			b.Height,
			tolerance,
		)
}

func approximatelyEqual(
	a float64,
	b float64,
	tolerance float64,
) bool {
	return math.Abs(a-b) <= tolerance
}
