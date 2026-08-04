package main

import (
	"flag"
	"fmt"
	"html"
	"math"
	"os"
	"sort"

	"github.com/HritikR/gopdfx/pdf"
)

func main() {
	password := flag.String("password", "", "Password to decrypt the PDF (if encrypted)")
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		fmt.Println("Usage: go run examples/main.go [-password <password>] <path-to-pdf>")
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
		err = doc.Unlock(*password)
		if err != nil {
			fmt.Printf("Failed to unlock document (incorrect or missing password): %v\n", err)
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

	for i := 0; i < numPages; i++ {
		page, err := doc.Page(i)
		if err != nil {
			fmt.Printf("Error loading page %d: %v\n", i, err)
			continue
		}

		elements, err := page.TextElements()
		if err != nil {
			fmt.Printf("Error extracting text elements from page %d: %v\n", i, err)
			continue
		}

		// Group elements into horizontal rows (Y within 2.0pt threshold)
		type row struct {
			y        float64
			elements []pdf.TextElement
		}
		var rows []row

		for _, el := range elements {
			found := false
			for j := range rows {
				if math.Abs(rows[j].y-el.Y) <= 2.0 {
					rows[j].elements = append(rows[j].elements, el)
					found = true
					break
				}
			}
			if !found {
				rows = append(rows, row{
					y:        el.Y,
					elements: []pdf.TextElement{el},
				})
			}
		}

		// Sort rows top-to-bottom (Y descending)
		sort.Slice(rows, func(a, b int) bool {
			return rows[a].y > rows[b].y
		})

		// Merge single-column rows with the row above based on X alignment
		var mergedRows []row

		for i, r := range rows {
			if len(r.elements) == 1 && i > 0 {
				el := r.elements[0]

				previousRow := &mergedRows[len(mergedRows)-1]

				merged := false

				for j, prevEl := range previousRow.elements {
					// Adjust threshold based on PDF precision
					if math.Abs(prevEl.X-el.X) <= 5.0 {
						// Same column, merge into previous row column
						previousRow.elements[j].Text += " " + el.Text
						merged = true
						break
					}
				}

				if merged {
					continue
				}
			}

			mergedRows = append(mergedRows, r)
		}

		rows = mergedRows

		// Write HTML file for the page
		htmlPath := fmt.Sprintf("output/page_%d.html", i+1)
		htmlFile, err := os.Create(htmlPath)
		if err != nil {
			fmt.Printf("Error creating HTML file: %v\n", err)
			continue
		}

		htmlFile.WriteString("<!DOCTYPE html>\n<html>\n<head>\n<meta charset=\"utf-8\">\n<style>\n")
		htmlFile.WriteString("body { font-family: sans-serif; background-color: #f7f9fc; margin: 20px; }\n")
		htmlFile.WriteString("table { border-collapse: collapse; width: 100%; box-shadow: 0 4px 6px rgba(0,0,0,0.1); background-color: #fff; margin-bottom: 30px; }\n")
		htmlFile.WriteString("td { border: 1px solid #e2e8f0; padding: 10px; font-family: monospace; font-size: 13px; color: #1a202c; }\n")
		htmlFile.WriteString("tr:nth-child(even) { background-color: #f8fafc; }\n")
		htmlFile.WriteString("</style>\n</head>\n<body>\n")
		fmt.Fprintf(htmlFile, "<h2>Page %d Table Layout</h2>\n", i+1)
		htmlFile.WriteString("<table>\n")

		for _, r := range rows {
			htmlFile.WriteString("  <tr>\n")
			for _, el := range r.elements {
				escaped := html.EscapeString(el.Text)
				fmt.Fprintf(htmlFile, "    <td>%s <span style=\"font-size: 12px; color: grey;\">(x: %.2f, y: %.2f)</span></td>\n", escaped, el.X, el.Y)
			}
			htmlFile.WriteString("  </tr>\n")
		}

		htmlFile.WriteString("</table>\n</body>\n</html>\n")
		htmlFile.Close()
		fmt.Printf("Successfully exported page %d table to: %s\n", i+1, htmlPath)
	}
}
