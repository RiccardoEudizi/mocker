package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"mocker/internal/parser"
	"os"
	"strings"

	"google.golang.org/genai"
)

// Generate mock data using an LLM
func GenerateMockDataLLM(td *parser.TypeDetails) (map[string]interface{}, error) {
	if os.Getenv("GOOGLE_API_KEY") == "" {
		return nil, fmt.Errorf("The GOOGLE_API_KEY must be set.")
	}

	result := make(map[string]interface{})

	if td == nil {
		return result, nil
	}
	schema := createSchema(td.Fields)

	err := generateObject(schema, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func createSchema(fields []parser.Field) map[string]*genai.Schema {
	result := make(map[string]*genai.Schema)
	for _, v := range fields {
		result[v.Name] = mapFieldToGenaiSchema(v)
	}

	return result
}

func mapFieldToGenaiSchema(field parser.Field) *genai.Schema {
	schema := &genai.Schema{}

	if field.IsCollection || len(field.GenericArgs) > 0 {
		schema.Type = genai.TypeArray

		var itemsSchema *genai.Schema

		if len(field.GenericArgs) > 0 {
			itemsSchema = &genai.Schema{Type: mapTypeToGenaiSchema(field.GenericArgs[0])}
		} else if strings.HasSuffix(field.Type, "[]") {
			elemType := strings.TrimSuffix(field.Type, "[]")
			itemsSchema = &genai.Schema{Type: mapTypeToGenaiSchema(elemType)}
		} else {
			itemsSchema = &genai.Schema{}
		}

		if field.TypeDetails != nil && len(field.TypeDetails.Fields) > 0 {
			itemsSchema.Properties = createSchema(field.TypeDetails.Fields)
			itemsSchema.Type = genai.TypeObject
		}

		schema.Items = itemsSchema
		return schema
	}

	schema.Type = mapTypeToGenaiSchema(field.Type)

	if field.TypeDetails != nil && len(field.TypeDetails.Fields) > 0 {
		schema.Properties = createSchema(field.TypeDetails.Fields)
	}

	return schema
}

func mapTypeToGenaiSchema(typeName string) genai.Type {
	simpleName := typeName
	if idx := strings.LastIndex(typeName, "."); idx >= 0 {
		simpleName = typeName[idx+1:]
	}

	switch strings.ToLower(simpleName) {
	case "string", "character", "char":
		return genai.TypeString
	case "integer", "int", "long", "short", "byte":
		return genai.TypeInteger
	case "float", "double", "bigdecimal", "biginteger":
		return genai.TypeNumber
	case "boolean", "bool":
		return genai.TypeBoolean
	default:
		return genai.TypeObject
	}
}

func generateObject(schema map[string]*genai.Schema, obj *map[string]any) error {
	ctx := context.Background()

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: os.Getenv("GEMINI_API_KEY"),
	})
	if err != nil {
		return fmt.Errorf("failed to create genai client: %w", err)
	}
	reqKeys := make([]string, len(schema))

	for k := range schema {
		reqKeys = append(reqKeys, k)
	}
	gschema := &genai.Schema{
		Type:       genai.TypeObject,
		Properties: schema,
	}

	config := &genai.GenerateContentConfig{
		ResponseSchema:   gschema,
		ResponseMIMEType: "application/json",
	}

	res, err := client.Models.GenerateContent(ctx, "gemini-flash-lite-latest",
		genai.Text("Generate a json object according to the schema."), config)

	if err != nil {
		return fmt.Errorf("failed to generate content: %w", err)
	}
	err = json.Unmarshal([]byte(res.Text()), obj)
	if err != nil {
		return err
	}

	return nil
}

func GenerateMockDataArrayLLM(td *parser.TypeDetails, count int) ([]map[string]interface{}, error) {
	if os.Getenv("GOOGLE_API_KEY") == "" {
		return nil, fmt.Errorf("The GOOGLE_API_KEY must be set.")
	}

	if td == nil {
		return []map[string]interface{}{}, nil
	}

	schema := createSchema(td.Fields)

	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: os.Getenv("GEMINI_API_KEY"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create genai client: %w", err)
	}

	itemSchema := &genai.Schema{
		Type:       genai.TypeObject,
		Properties: schema,
	}

	gschema := &genai.Schema{
		Type:  genai.TypeArray,
		Items: itemSchema,
	}

	config := &genai.GenerateContentConfig{
		ResponseSchema:   gschema,
		ResponseMIMEType: "application/json",
	}

	res, err := client.Models.GenerateContent(ctx, "gemini-flash-lite-latest",
		genai.Text(fmt.Sprintf("Generate a json array with exactly %d distinct objects according to the schema. Each object should be different.", count)), config)

	if err != nil {
		return nil, fmt.Errorf("failed to generate content: %w", err)
	}

	var result []map[string]interface{}
	err = json.Unmarshal([]byte(res.Text()), &result)
	if err != nil {
		return nil, err
	}

	return result, nil
}
