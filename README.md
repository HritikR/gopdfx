# gopdfx

`gopdfx` is a Go PDF parsing library focused on reading PDF structure,
extracting text, and inspecting page graphics.

## Install

```bash
go get github.com/HritikR/gopdfx
```

## Basic usage

```go
package main

import (
	"fmt"
	"log"

	"github.com/HritikR/gopdfx/pdf"
)

func main() {
	doc, err := pdf.Open("statement.pdf")
	if err != nil {
		log.Fatal(err)
	}

	text, err := doc.Text()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(text)
}
```

## Positioned text

For fixed-layout documents, use page text elements instead of relying only on
the reconstructed plain-text output:

```go
page, err := doc.Page(0)
if err != nil {
	log.Fatal(err)
}

elements, err := page.TextElements()
if err != nil {
	log.Fatal(err)
}

for _, element := range elements {
	fmt.Printf("x=%.2f y=%.2f text=%q\n", element.X, element.Y, element.Text)
}
```

`TextElements` returns text runs in page coordinates. Applications can group
those runs into lines or columns according to the layout of the document.
Bank-specific transaction interpretation belongs in the consuming application,
not in this generic PDF package.

## Encrypted PDFs

```go
if doc.Encryption.IsActive() {
	if err := doc.Unlock(password); err != nil {
		log.Fatal(err)
	}
}
```

The library currently supports the PDF encryption revisions implemented in the
`crypto` package. Unsupported encryption algorithms return an error.

## Supported functionality

- Classic cross-reference tables and PDF cross-reference streams
- Incremental updates and compressed object streams
- PDF arrays, dictionaries, strings, names, and streams
- Flate, ASCII hex, ASCII85, and run-length stream filters
- Page-tree traversal
- Text extraction with basic font and `ToUnicode` CMap support
- Positioned text elements
- Basic page graphics and ruled-line detection
- RC4 and supported AES PDF encryption modes

## Limitations

This is a parser, not a PDF renderer or OCR engine. Some PDFs require
additional application-level processing, including scanned/image-only PDFs,
complex font encodings, unsupported stream filters, Form XObjects, and
bank-specific transaction layouts.

Always validate extracted financial data independently, for example by
checking running balances and transaction totals.

## Development

```bash
go test ./...
go vet ./...
```

The example program writes diagnostic text and HTML files to `output/`:

```bash
go run ./examples ./path/to/file.pdf
```
