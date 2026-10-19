<p align="center">
  <img src="assets/pdfopher.png" alt="gopdfx PDFopher mascot" width="120">
</p>

<p align="center">
  <strong>gopdfx - A PDF reader library for Go.</strong>
  <br>   
<p align="center">Read PDF objects, extract text, inspect page graphics, and work with supported encrypted documents.</p>
</p>

## Install

Run this command:

```bash
go get github.com/HritikR/gopdfx
```

## Read a PDF

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

## Read positioned text

Use `TextElements` when the PDF uses fixed page positions.

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

Each element has a page position and a text value.
The application can group elements into lines and columns.
The application must handle document-specific layouts.

## Read an encrypted PDF

```go
if doc.Encryption.IsActive() {
	if err := doc.Unlock(password); err != nil {
		log.Fatal(err)
	}
}
```

The `crypto` package supports the encryption modes implemented by this
library.
The library returns an error for an unsupported mode.

## Supported features

- Classic cross-reference tables.
- Cross-reference streams.
- Incremental PDF updates.
- Compressed object streams.
- PDF arrays, dictionaries, strings, names, and streams.
- Flate, ASCII hex, ASCII85, and run-length stream filters.
- Page-tree traversal.
- Basic font and `ToUnicode` CMap support.
- Positioned text elements.
- Basic page graphics.
- Ruled-line detection.
- Supported RC4 and AES PDF encryption modes.

## Limits

This library is not a PDF renderer.
This library does not perform OCR.
An image-only PDF needs an OCR tool.
Some PDFs use unsupported filters, fonts, or content structures.
The application must check extracted financial data.
For example, it can check running balances and transaction totals.

## Development

Run the tests:

```bash
go test ./...
```

Run static checks:

```bash
go vet ./...
```

Run the example program:

```bash
go run ./examples ./path/to/file.pdf
```

example code writes diagnostic text and HTML files to `output/`.
