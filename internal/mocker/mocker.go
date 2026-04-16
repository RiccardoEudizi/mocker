package mocker

import (
	"fmt"
	"strings"
	"time"

	"mocker/internal/ai"
	"mocker/internal/parser"

	"github.com/jaswdr/faker"
)

type Generator struct {
	useLocal      bool
	faker         faker.Faker
	collectionLen int
}

func New(useLocal bool) *Generator {
	return &Generator{
		useLocal:      useLocal,
		faker:         faker.New(),
		collectionLen: 10,
	}
}

func (g *Generator) GenerateMockForEndpoint(endpoint *parser.Endpoint) interface{} {
	if endpoint.TypeDetails == nil {
		return nil
	}

	if isListEndpoint(endpoint) {
		return g.generateList(endpoint.TypeDetails)
	}

	item := g.GenerateMockFromTypeDetails(endpoint.TypeDetails)
	return item
}

func (g *Generator) GenerateMockFromTypeDetails(td *parser.TypeDetails) map[string]interface{} {

	if td == nil {
		return nil
	}

	if g.useLocal {
		return g.generateFromTypeDetailsFaker(td)
	}

	obj, err := ai.GenerateMockDataLLM(td)
	if err != nil {
		fmt.Println(err)
		return nil
	}

	return obj
}

func (g *Generator) generateFromTypeDetailsFaker(td *parser.TypeDetails) map[string]interface{} {
	result := make(map[string]interface{})
	for _, field := range td.Fields {
		result[field.Name] = g.generateFieldValue(field)
	}
	return result
}

func (g *Generator) generateFieldValue(field parser.Field) interface{} {
	typeName := field.Type

	if field.IsCollection {
		return g.generateCollection(field)
	}

	typeName = extractBaseType(typeName)

	switch {
	case strings.Contains(typeName, "String"):
		return g.generateStringValue(field.Name)
	case strings.Contains(typeName, "Integer"), strings.Contains(typeName, "Int"):
		return g.faker.IntBetween(1, 1000)
	case strings.Contains(typeName, "Long"):
		return int64(g.faker.IntBetween(1, 10000))
	case strings.Contains(typeName, "Double"), strings.Contains(typeName, "Float"):
		return g.faker.Float64(2, 0, 1000)
	case strings.Contains(typeName, "Boolean"):
		return g.faker.Bool()
	case strings.Contains(typeName, "Date"):
		return g.faker.Time().Time(time.Now()).Format(time.RFC3339)
	case strings.Contains(typeName, "UUID"):
		return g.faker.UUID().V4()
	case isPrimitiveType(typeName):
		return nil
	default:
		if field.TypeDetails != nil {
			return g.GenerateMockFromTypeDetails(field.TypeDetails)
		}
		return nil
	}
}

func (g *Generator) generateStringValue(fieldName string) string {
	fieldNameLower := strings.ToLower(fieldName)

	if strings.Contains(fieldNameLower, "email") {
		return g.faker.Internet().Email()
	}
	if strings.Contains(fieldNameLower, "phone") {
		return g.faker.Phone().Number()
	}
	if strings.Contains(fieldNameLower, "address") || strings.Contains(fieldNameLower, "street") {
		return g.faker.Address().StreetAddress()
	}
	if strings.Contains(fieldNameLower, "city") {
		return g.faker.Address().City()
	}
	if strings.Contains(fieldNameLower, "country") {
		return g.faker.Address().Country()
	}
	if strings.Contains(fieldNameLower, "zip") || strings.Contains(fieldNameLower, "postal") {
		return g.faker.Address().PostCode()
	}
	if strings.Contains(fieldNameLower, "name") && strings.Contains(fieldNameLower, "first") {
		return g.faker.Person().FirstName()
	}
	if strings.Contains(fieldNameLower, "surname") || strings.Contains(fieldNameLower, "lastname") {
		return g.faker.Person().LastName()
	}
	if strings.Contains(fieldNameLower, "name") || strings.Contains(fieldNameLower, "title") {
		return g.faker.Lorem().Text(10)
	}
	if strings.Contains(fieldNameLower, "description") || strings.Contains(fieldNameLower, "label") {
		return g.faker.Lorem().Sentence(5)
	}
	if strings.Contains(fieldNameLower, "id") {
		return fmt.Sprintf("%d", g.faker.IntBetween(1, 1000))
	}
	if strings.Contains(fieldNameLower, "code") {
		return fmt.Sprintf("CODE-%d", g.faker.IntBetween(100, 999))
	}
	if strings.Contains(fieldNameLower, "url") || strings.Contains(fieldNameLower, "link") {
		return g.faker.Internet().URL()
	}
	if strings.Contains(fieldNameLower, "password") {
		return g.faker.Lorem().Text(10)
	}
	if strings.Contains(fieldNameLower, "username") {
		return g.faker.Internet().User()
	}

	return g.faker.Lorem().Word()
}

func (g *Generator) generateCollection(field parser.Field) []interface{} {
	length := g.collectionLen
	if len(field.GenericArgs) > 0 {
		argType := field.GenericArgs[0]
		argType = extractBaseType(argType)

		if isPrimitiveType(argType) {
			return g.generatePrimitiveCollection(argType, length)
		}

		if field.TypeDetails != nil {
			if g.useLocal {
				result := make([]interface{}, length)
				for i := 0; i < length; i++ {
					result[i] = g.generateFromTypeDetailsFaker(field.TypeDetails)
				}
				return result
			}
			items, err := ai.GenerateMockDataArrayLLM(field.TypeDetails, length)
			if err != nil {
				fmt.Println(err)
				return nil
			}
			result := make([]interface{}, len(items))
			for i, item := range items {
				result[i] = item
			}
			return result
		}

		items := make([]interface{}, length)
		for i := 0; i < length; i++ {
			items[i] = map[string]interface{}{
				"id": g.faker.IntBetween(1, 1000),
			}
		}
		return items
	}

	return nil
}

func (g *Generator) generatePrimitiveCollection(typeName string, length int) []interface{} {
	items := make([]interface{}, length)
	for i := 0; i < length; i++ {
		switch {
		case strings.Contains(typeName, "String"):
			items[i] = g.faker.Lorem().Word()
		case strings.Contains(typeName, "Integer"), strings.Contains(typeName, "Int"):
			items[i] = g.faker.IntBetween(1, 100)
		case strings.Contains(typeName, "Long"):
			items[i] = int64(g.faker.IntBetween(1, 1000))
		case strings.Contains(typeName, "Double"), strings.Contains(typeName, "Float"):
			items[i] = g.faker.Float64(2, 0, 100)
		case strings.Contains(typeName, "Boolean"):
			items[i] = g.faker.Bool()
		default:
			items[i] = nil
		}
	}
	return items
}

func (g *Generator) generateList(td *parser.TypeDetails) []map[string]interface{} {
	if g.useLocal {
		return g.generateListFaker(td)
	}

	items, err := ai.GenerateMockDataArrayLLM(td, g.collectionLen)
	if err != nil {
		fmt.Println(err)
		return nil
	}
	return items
}

func (g *Generator) generateListFaker(td *parser.TypeDetails) []map[string]interface{} {
	items := make([]map[string]interface{}, g.collectionLen)
	for i := 0; i < g.collectionLen; i++ {
		items[i] = g.generateFromTypeDetailsFaker(td)
	}
	return items
}

func isListEndpoint(endpoint *parser.Endpoint) bool {
	if endpoint.Method != "GET" {
		return false
	}

	returnType := endpoint.ReturnType
	if strings.Contains(returnType, "List") || strings.Contains(returnType, "Collection") {
		return true
	}

	if endpoint.TypeDetails != nil && len(endpoint.TypeDetails.Fields) > 0 {
		if endpoint.TypeDetails.IsCollection {
			return true
		}
	}

	return false
}

func extractBaseType(typeName string) string {
	if idx := strings.Index(typeName, "<"); idx > 0 {
		return typeName[:idx]
	}
	return typeName
}

func isPrimitiveType(typeName string) bool {
	primitives := []string{
		"String", "Integer", "Long", "Boolean", "Double", "Float",
		"Byte", "Short", "Char", "Date", "UUID",
	}

	typeName = extractBaseType(typeName)
	typeName = strings.TrimPrefix(typeName, "java.lang.")
	typeName = strings.TrimPrefix(typeName, "java.util.")

	for _, p := range primitives {
		if typeName == p {
			return true
		}
	}
	return false
}

func sanitizeFilename(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, "{", "")
	name = strings.ReplaceAll(name, "}", "")

	runes := []rune(name)
	var result []rune
	for _, r := range runes {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result = append(result, r)
		}
	}

	// Clean up multiple dashes
	resultStr := string(result)
	for strings.Contains(resultStr, "--") {
		resultStr = strings.ReplaceAll(resultStr, "--", "-")
	}
	// Remove leading/trailing dashes
	resultStr = strings.Trim(resultStr, "-")

	return resultStr
}

func GenerateFilename(endpoint *parser.Endpoint) string {
	method := endpoint.Method
	path := sanitizeFilename(endpoint.Path)
	handler := sanitizeFilename(endpoint.Handler)

	return fmt.Sprintf("%s-%s-%s.json", method, path, handler)
}
