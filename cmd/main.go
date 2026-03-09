package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"mocker/internal/mocker"
	"mocker/internal/parser"
	"mocker/internal/server"

	"github.com/joho/godotenv"
)

func main() {

	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	srcDir := flag.String("src", ".", "Source directory to scan for imported files")
	mock := flag.Bool("mock", false, "Generate mock JSON files for each endpoint")
	mockDir := flag.String("mock-dir", "mocks", "Directory to store mock files")
	generateServer := flag.Bool("server", false, "Generate Gin server with mock endpoints")
	serverDir := flag.String("server-dir", "server", "Directory to store server files")
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "Usage: %s [-src <source-directory>] <path-to-java-controller-file>\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(1)
	}

	// If server is requested, implicitly enable mock generation
	if *generateServer {
		*mock = true
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

	if *mock {
		gen := mocker.New()

		err = os.MkdirAll(*mockDir, 0755)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating mock directory: %v\n", err)
			os.Exit(1)
		}

		for i := range result.Endpoints {
			endpoint := &result.Endpoints[i]
			filename := mocker.GenerateFilename(endpoint)
			mockData := gen.GenerateMockForEndpoint(endpoint)

			if mockData == nil {
				continue
			}

			mockJSON, err := json.MarshalIndent(mockData, "", "  ")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error marshaling mock data for %s: %v\n", endpoint.Path, err)
				continue
			}

			mockPath := filepath.Join(*mockDir, filename)
			err = os.WriteFile(mockPath, mockJSON, 0644)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error writing mock file %s: %v\n", mockPath, err)
				continue
			}

			fmt.Println(mockPath)
		}

		// Generate server if requested
		if *generateServer {
			serverGen := server.New(*mockDir, *serverDir)
			if err := serverGen.Generate(result); err != nil {
				fmt.Fprintf(os.Stderr, "Error generating server: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Server generated in %s/\n", *serverDir)
			fmt.Printf("Run with: cd %s && go run main.go\n", *serverDir)
		}
	}
}
