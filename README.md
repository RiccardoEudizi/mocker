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

Generates mock JSON files for each endpoint in `out/mocks/`.

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
- Generates realistic mock data using faker library
- Handles primitive types, collections, and nested objects
- Creates properly named files based on endpoint paths

### Server Generation
- Generates a complete Gin HTTP server
- Serves mock data via REST endpoints
- Matches the original controller's paths and methods

## Requirements

- Go 1.25+
- tree-sitter Java grammar