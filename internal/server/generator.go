package server

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"mocker/internal/parser"
)

type Generator struct {
	mockDir   string
	serverDir string
}

func New(mockDir, serverDir string) *Generator {
	return &Generator{
		mockDir:   mockDir,
		serverDir: serverDir,
	}
}

func (g *Generator) Generate(result *parser.Result) error {
	dataDir := filepath.Join(g.serverDir, "data")

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create server directory: %w", err)
	}

	if err := g.copyMockFiles(dataDir); err != nil {
		return fmt.Errorf("failed to copy mock files: %w", err)
	}

	if err := g.generateMainGo(result); err != nil {
		return fmt.Errorf("failed to generate main.go: %w", err)
	}

	if err := g.generateGoMod(); err != nil {
		return fmt.Errorf("failed to generate go.mod: %w", err)
	}

	// Run go mod tidy to download dependencies
	if err := g.runGoModTidy(); err != nil {
		return fmt.Errorf("failed to run go mod tidy: %w", err)
	}

	return nil
}

func (g *Generator) runGoModTidy() error {
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = g.serverDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (g *Generator) copyMockFiles(dataDir string) error {
	files, err := os.ReadDir(g.mockDir)
	if err != nil {
		return fmt.Errorf("failed to read mock directory: %w", err)
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		src := filepath.Join(g.mockDir, file.Name())
		dst := filepath.Join(dataDir, file.Name())

		data, err := os.ReadFile(src)
		if err != nil {
			return fmt.Errorf("failed to read mock file %s: %w", file.Name(), err)
		}

		if err := os.WriteFile(dst, data, 0644); err != nil {
			return fmt.Errorf("failed to write mock file %s: %w", file.Name(), err)
		}
	}

	return nil
}

func (g *Generator) generateMainGo(result *parser.Result) error {
	mockFilesMap := g.generateMockFilesMap(result)

	tmpl, err := template.New("main.go").Parse(mainGoTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	data := struct {
		Filename  string
		BasePath  string
		MockFiles string
	}{
		Filename:  result.Filename,
		BasePath:  result.BasePath,
		MockFiles: mockFilesMap,
	}

	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	mainGoPath := filepath.Join(g.serverDir, "main.go")
	if err := os.WriteFile(mainGoPath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write main.go: %w", err)
	}

	return nil
}

func (g *Generator) generateMockFilesMap(result *parser.Result) string {
	seenPaths := make(map[string]bool)
	var lines []string

	for _, endpoint := range result.Endpoints {
		mockFile := getMockFilename(&endpoint)
		path := endpoint.Path

		mockFilePath := filepath.Join(g.mockDir, mockFile)
		if _, err := os.Stat(mockFilePath); err != nil {
			continue
		}

		if endpoint.Method == "GET" || endpoint.Method == "POST" ||
			endpoint.Method == "PUT" || endpoint.Method == "DELETE" ||
			endpoint.Method == "PATCH" {

			if mockFile != "" && path != "" {
				path = strings.TrimSuffix(path, "/")

				// Skip if we've already seen this path
				if seenPaths[path] {
					continue
				}
				seenPaths[path] = true

				lines = append(lines, fmt.Sprintf("    %q: %q,", path, "data/"+mockFile))
			}
		}
	}

	return strings.Join(lines, "\n")
}

func (g *Generator) generateGoMod() error {
	tmpl, err := template.New("go.mod").Parse(goModTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse go.mod template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, nil); err != nil {
		return fmt.Errorf("failed to execute go.mod template: %w", err)
	}

	goModPath := filepath.Join(g.serverDir, "go.mod")
	if err := os.WriteFile(goModPath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write go.mod: %w", err)
	}

	return nil
}

func getMockFilename(endpoint *parser.Endpoint) string {
	method := endpoint.Method
	path := sanitizePath(endpoint.Path)
	handler := sanitizePath(endpoint.Handler)

	if path == "" {
		return fmt.Sprintf("%s-%s-%s.json", method, handler, handler)
	}

	return fmt.Sprintf("%s-%s-%s.json", method, path, handler)
}

func sanitizePath(path string) string {
	path = strings.ToLower(path)
	path = strings.ReplaceAll(path, "/", "-")
	path = strings.ReplaceAll(path, "{", "")
	path = strings.ReplaceAll(path, "}", "")

	var result []rune
	for _, r := range path {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result = append(result, r)
		}
	}

	resultStr := string(result)
	for strings.Contains(resultStr, "--") {
		resultStr = strings.ReplaceAll(resultStr, "--", "-")
	}
	return strings.Trim(resultStr, "-")
}

var mainGoTemplate = `package main

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

var mockFiles = map[string]string{
{{.MockFiles}}
}

func newRouter() *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger())
	r.Use(gin.Recovery())

	// CORS middleware
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	})

	// Generic handler that matches path to mock file
	r.Use(func(c *gin.Context) {
		path := c.Request.URL.Path

		// Try exact match first
		if file, ok := mockFiles[path]; ok {
			data, _ := os.ReadFile(file)
			var jsonData interface{}
			json.Unmarshal(data, &jsonData)
			c.JSON(200, jsonData)
			c.Abort()
			return
		}

		// Try matching with path parameters
		for pattern, file := range mockFiles {
			if matchPath(pattern, path) {
				data, _ := os.ReadFile(file)
				var jsonData interface{}
				json.Unmarshal(data, &jsonData)
				c.JSON(200, jsonData)
				c.Abort()
				return
			}
		}

		c.Next()
	})

	// Serve static files from data directory
	r.Static("/data", "./data")

	return r
}

func matchPath(pattern, path string) bool {
	// Simple pattern matching for path parameters
	// e.g., /users/{id} matches /users/123
	patternParts := strings.Split(pattern, "/")
	pathParts := strings.Split(path, "/")

	if len(patternParts) != len(pathParts) {
		return false
	}

	for i, part := range patternParts {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			// This is a path parameter, matches anything
			continue
		}
		if part != pathParts[i] {
			return false
		}
	}

	return true
}

// Generated from {{.Filename}}
// Base path: {{.BasePath}}

func main() {
	r := newRouter()
	r.Run()
}
`

var goModTemplate = `module server

go 1.22

require github.com/gin-gonic/gin v1.10.0
`
