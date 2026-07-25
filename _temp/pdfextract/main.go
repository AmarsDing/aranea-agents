package main

import (
	"fmt"
	"os"

	"github.com/ledongthuc/pdf"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("usage: pdfextract <in.pdf> <out.txt>")
		os.Exit(1)
	}
	f, r, err := pdf.Open(os.Args[1])
	if err != nil {
		fmt.Println("open:", err)
		os.Exit(1)
	}
	defer f.Close()

	out, err := os.Create(os.Args[2])
	if err != nil {
		fmt.Println("create:", err)
		os.Exit(1)
	}
	defer out.Close()

	total := r.NumPage()
	for i := 1; i <= total; i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			continue
		}
		text, err := p.GetPlainText(nil)
		if err != nil {
			fmt.Fprintf(out, "\n=== page %d (err: %v) ===\n", i, err)
			continue
		}
		fmt.Fprintf(out, "\n=== page %d ===\n%s\n", i, text)
	}
	fmt.Println("pages:", total)
}
