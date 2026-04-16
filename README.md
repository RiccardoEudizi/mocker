# Mocker - Java JAX-RS Controller Parser & Mock Generator

A Go CLI tool that parses Java JAX-RS controller files using tree-sitter AST parsing, extracts endpoint information, and generates mock JSON data for testing.

## Installation

```bash
go build -o mocker ./cmd/main.go
```

## Usage

```bash
./mocker [flags] <path-to-java-controller-file>
```

### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-src` | string | `.` | Source directory to scan for imported files |
| `-mock` | bool | `false` | Generate mock JSON files for each endpoint |
| `-mock-dir` | string | `out/mocks` | Directory to store mock files |
| `-server` | bool | `false` | Generate Gin server with mock endpoints |
| `-server-dir` | string | `out/server` | Directory to store server files |
| `-local` | bool | `false` | Use local faker instead of AI for mock generation |

### Arguments

- `path-to-java-controller-file` (required): Path to the Java controller file

## Commands

### Parse Controller

```bash
./mocker com/example/UserController.java
```

Reads the Java controller and outputs `endpoints.json` with endpoint details.

### Generate Mocks

```bash
./mocker -mock com/example/UserController.java
```

Generates mock JSON files for each endpoint in `out/mocks/`. Uses AI (Google Gemini) by default. Use `-local` to use local faker instead.

### Generate Mocks (Local)

```bash
./mocker -mock -local com/example/UserController.java
```

Generates mock JSON files using local faker library (no API key required).

### Generate Mock Server

```bash
./mocker -server com/example/UserController.java
```

Generates a complete Gin server with mock endpoints in `out/server/`. Run it with:

```bash
cd out/server && go run main.go
```

### Example

```bash
# Parse controller
./mocker -src src/main/java com/example/UserController.java

# Generate mocks
./mocker -mock -src src/main/java com/example/UserController.java

# Generate and run mock server
./mocker -server -src src/main/java com/example/UserController.java
cd out/server && go run main.go
```

## Features

### JAX-RS Annotation Support
- `@Path` (class and method level)
- `@GET`, `@POST`, `@PUT`, `@DELETE`, `@PATCH`, `@HEAD`, `@OPTIONS`
- `@Produces`, `@Consumes`

### Mock Generation
- Uses AI (Google Gemini) by default to generate realistic mock data
- Use `-local` flag for local faker generation (no API key required)
- Handles primitive types, collections, and nested objects
- Creates properly named files based on endpoint paths

### Server Generation
- Generates a complete Gin HTTP server
- Serves mock data via REST endpoints
- Matches the original controller's paths and methods

## Requirements

- Go 1.25+
- tree-sitter Java grammar

## Environment Variables

When using AI (default):
- `GEMINI_API_KEY`: Google Gemini API key