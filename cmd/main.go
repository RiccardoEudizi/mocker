package main

import (
	"flag"
	"fmt"
	"os"

	"mocker/internal/parser"
)

func main() {
	srcDir := flag.String("src", ".", "Source directory to scan for imported files")
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "Usage: %s [-src <source-directory>] <path-to-java-controller-file>\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(1)
	}

	filePath := flag.Arg(0)

	p := parser.New(*srcDir)
	defer p.Close()

	result, err := p.Parse(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	jsonData, err := result.ToJSON()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling JSON: %v\n", err)
		os.Exit(1)
	}

	err = os.WriteFile("endpoints.json", jsonData, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing file: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("endpoints.json")
}
