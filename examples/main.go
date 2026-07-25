package main

import (
	"flag"
	"fmt"
	"os"

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
		fmt.Println("Provided password:", *password)
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

		text, err := page.Text()
		if err != nil {
			fmt.Printf("Error extracting text from page %d: %v\n", i, err)
			continue
		}

		fmt.Printf("--- Page %d ---\n", i+1)
		fmt.Println(text)
	}
}
