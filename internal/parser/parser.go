package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	java "github.com/tree-sitter/tree-sitter-java/bindings/go"
)

type importInfo struct {
	fullName   string
	simpleName string
	isStatic   bool
	filePath   string
}

type Parser struct {
	parser       *sitter.Parser
	lang         *sitter.Language
	srcDir       string
	imports      []importInfo
	primitives   map[string]bool
	javaLang     map[string]bool
	typeCache    map[string]*TypeDetails
	packageCache map[string]string
	importCache  map[string][]importInfo
	currentPkg   string
	localClasses map[string]string
}

func New(srcDir string) *Parser {
	lang := sitter.NewLanguage(java.Language())
	p := sitter.NewParser()
	p.SetLanguage(lang)

	primitives := map[string]bool{
		"void": true, "boolean": true, "byte": true, "short": true,
		"int": true, "long": true, "float": true, "double": true, "char": true,
	}

	javaLang := map[string]bool{
		"String": true, "Integer": true, "Long": true, "Boolean": true,
		"Double": true, "Float": true, "Byte": true, "Short": true,
		"Character": true, "Object": true, "Class": true, "Runnable": true,
		"Comparable": true, "Iterable": true, "Collection": true,
		"Optional": true, "OptionalInt": true, "OptionalLong": true,
		"BigDecimal": true, "BigInteger": true, "Date": true, "UUID": true,
		"Exception": true, "RuntimeException": true, "Throwable": true,
		"InputStream": true, "OutputStream": true, "Reader": true, "Writer": true,
		"BufferedReader": true, "BufferedWriter": true, "File": true, "Path": true,
	}

	for k, v := range javaLang {
		primitives[k] = v
	}

	return &Parser{
		parser:       p,
		lang:         lang,
		srcDir:       srcDir,
		imports:      make([]importInfo, 0),
		primitives:   primitives,
		javaLang:     javaLang,
		typeCache:    make(map[string]*TypeDetails),
		packageCache: make(map[string]string),
		importCache:  make(map[string][]importInfo),
		localClasses: make(map[string]string),
	}
}

func (p *Parser) Parse(filePath string) (*Result, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	tree := p.parser.Parse(nil, content)
	if tree == nil {
		return nil, fmt.Errorf("failed to parse")
	}
	defer tree.Close()

	root := tree.RootNode()

	p.currentPkg = p.extractPackage(root, content)
	p.localClasses = p.scanLocalClasses(filePath)

	result := &Result{
		Filename:  filepath.Base(filePath),
		Endpoints: make([]Endpoint, 0),
	}

	p.imports = p.collectImports(root, content)

	classNode := p.findClassDeclaration(root)
	if classNode == nil {
		return result, nil
	}

	p.parseClassAnnotations(classNode, content, result)

	methodNodes := p.findMethodDeclarations(classNode)
	for _, methodNode := range methodNodes {
		endpoint := p.parseMethod(methodNode, content, result)
		if endpoint != nil {
			result.Endpoints = append(result.Endpoints, *endpoint)
		}
	}

	return result, nil
}

func (p *Parser) extractPackage(root *sitter.Node, content []byte) string {
	iter := sitter.NewIterator(root, sitter.DFSMode)
	for {
		node, err := iter.Next()
		if err != nil || node == nil {
			break
		}
		if node.Type() == "package_declaration" {
			scopedIdent := p.findChildByType(node, "scoped_identifier")
			if scopedIdent != nil {
				return string(content[scopedIdent.StartByte():scopedIdent.EndByte()])
			}
		}
	}
	return ""
}

func (p *Parser) scanLocalClasses(controllerFilePath string) map[string]string {
	classes := make(map[string]string)

	controllerDir := filepath.Dir(controllerFilePath)

	entries, err := os.ReadDir(controllerDir)
	if err != nil {
		return classes
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".java") {
			continue
		}

		className := strings.TrimSuffix(name, ".java")
		if className == "package-info" {
			continue
		}

		classFilePath := filepath.Join(controllerDir, name)
		classes[className] = classFilePath
	}

	return classes
}

func (p *Parser) collectImports(root *sitter.Node, content []byte) []importInfo {
	var imports []importInfo

	iter := sitter.NewIterator(root, sitter.DFSMode)
	for {
		node, err := iter.Next()
		if err != nil || node == nil {
			break
		}
		if node.Type() == "import_declaration" {
			importDecl := p.parseImportDeclaration(node, content)
			if importDecl.fullName != "" {
				imports = append(imports, importDecl)
			}
		}
	}

	return imports
}

func (p *Parser) parseImportDeclaration(node *sitter.Node, content []byte) importInfo {
	var info importInfo

	childCount := int(node.ChildCount())
	if childCount < 2 {
		return info
	}

	firstChild := node.Child(0)
	if firstChild != nil && firstChild.Type() == "annotation" {
		return info
	}

	var identNode *sitter.Node
	for i := childCount - 1; i >= 0; i-- {
		child := node.Child(i)
		if child != nil && (child.Type() == "scoped_identifier" || child.Type() == "identifier") {
			identNode = child
			break
		}
	}

	if identNode == nil {
		return info
	}

	fullName := string(content[identNode.StartByte():identNode.EndByte()])
	info.fullName = fullName
	info.simpleName = fullName[strings.LastIndex(fullName, ".")+1:]

	isWildcard := false
	for i := 0; i < childCount-1; i++ {
		child := node.Child(i)
		if child != nil && child.Type() == "asterisk" {
			isWildcard = true
			break
		}
	}

	if !isWildcard {
		info.filePath = p.findSourceFile(info.fullName)
	}

	for i := 0; i < childCount-1; i++ {
		child := node.Child(i)
		if child != nil && child.Type() == "keyword" && child.Content(content) == "static" {
			info.isStatic = true
			break
		}
	}

	return info
}

func (p *Parser) findSourceFile(fullyQualifiedName string) string {
	packagePath := strings.ReplaceAll(fullyQualifiedName, ".", string(filepath.Separator))
	extensions := []string{".java", ".JAVA"}

	// First try direct path
	for _, ext := range extensions {
		filePath := filepath.Join(p.srcDir, packagePath+ext)
		if _, err := os.Stat(filePath); err == nil {
			return filePath
		}
	}

	// Search recursively in srcDir for files matching the package structure
	className := filepath.Base(packagePath) + ".java"
	packageDirs := filepath.Dir(packagePath)

	var foundPath string
	filepath.WalkDir(p.srcDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if strings.EqualFold(d.Name(), className) {
			// Check if this file is in the correct package directory
			dir := filepath.Dir(path)
			if strings.HasSuffix(dir, packageDirs) || dir == packageDirs {
				foundPath = path
				return filepath.SkipAll
			}
		}
		return nil
	})

	return foundPath
}

func (p *Parser) parseSourceFile(filePath string) (*TypeDetails, error) {
	if filePath == "" {
		return nil, fmt.Errorf("empty file path")
	}

	if cached, ok := p.typeCache[filePath]; ok {
		return cached, nil
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	tree := p.parser.Parse(nil, content)
	if tree == nil {
		return nil, fmt.Errorf("failed to parse")
	}
	defer tree.Close()

	root := tree.RootNode()
	classNode := p.findClassDeclaration(root)
	if classNode == nil {
		return nil, fmt.Errorf("no class declaration found")
	}

	// Extract and cache the package and imports from this file
	filePkg := p.extractPackageWithCache(root, content, filePath)
	fileImports := p.extractImportsWithCache(root, content, filePath)

	// Save original context
	originalPkg := p.currentPkg
	originalImports := p.imports

	// Set bean file's context for type resolution
	if filePkg != "" {
		p.currentPkg = filePkg
	}
	if len(fileImports) > 0 {
		p.imports = fileImports
	}

	typeDetails := p.extractClassDetails(classNode, content)

	// Restore original context
	p.currentPkg = originalPkg
	p.imports = originalImports

	p.typeCache[filePath] = typeDetails

	return typeDetails, nil
}

func (p *Parser) extractPackageWithCache(root *sitter.Node, content []byte, filePath string) string {
	if pkg, ok := p.packageCache[filePath]; ok {
		return pkg
	}
	pkg := p.extractPackage(root, content)
	p.packageCache[filePath] = pkg
	return pkg
}

func (p *Parser) extractImportsWithCache(root *sitter.Node, content []byte, filePath string) []importInfo {
	if imports, ok := p.importCache[filePath]; ok {
		return imports
	}
	imports := p.collectImports(root, content)
	p.importCache[filePath] = imports
	return imports
}

func (p *Parser) extractClassDetails(classNode *sitter.Node, content []byte) *TypeDetails {
	details := &TypeDetails{
		Fields: make([]Field, 0),
	}

	nameNode := p.findChildByType(classNode, "identifier")
	if nameNode != nil {
		details.Name = string(content[nameNode.StartByte():nameNode.EndByte()])
	}

	packageNode := p.findPackageDeclaration(rootFromNode(classNode))
	if packageNode != nil {
		details.Package = string(content[packageNode.StartByte():packageNode.EndByte()])
	}

	if details.Package != "" {
		details.FullName = details.Package + "." + details.Name
	} else {
		details.FullName = details.Name
	}

	extendsNode := p.findChildByType(classNode, "extends")
	if extendsNode != nil {
		typeNode := p.findChildByType(extendsNode, "type")
		if typeNode != nil {
			typeIdent := p.findChildByType(typeNode, "type_identifier")
			if typeIdent != nil {
				details.Extends = p.resolveTypeName(string(content[typeIdent.StartByte():typeIdent.EndByte()]))
			}
		}
	}

	implementsNode := p.findChildByType(classNode, "implements")
	if implementsNode != nil {
		typeList := p.findChildByType(implementsNode, "type_list")
		if typeList != nil {
			childCount := int(typeList.ChildCount())
			for i := 0; i < childCount; i++ {
				child := typeList.Child(i)
				if child != nil && child.Type() == "type" {
					typeIdent := p.findChildByType(child, "type_identifier")
					if typeIdent != nil {
						details.Implements = append(details.Implements,
							p.resolveTypeName(string(content[typeIdent.StartByte():typeIdent.EndByte()])))
					}
				}
			}
		}
	}

	classBody := p.findChildByType(classNode, "class_body")
	if classBody != nil {
		childCount := int(classBody.ChildCount())
		for i := 0; i < childCount; i++ {
			child := classBody.Child(i)
			if child != nil && child.Type() == "field_declaration" {
				field := p.extractField(child, content, 0)
				if field.Name != "" {
					details.Fields = append(details.Fields, field)
				}
			}
		}
	}

	return details
}

func (p *Parser) extractField(fieldNode *sitter.Node, content []byte, depth int) Field {
	field := Field{}

	if depth > 40 {
		return field
	}

	modifiers := p.findChildByType(fieldNode, "modifiers")
	if modifiers != nil {
		isStatic := false
		isFinal := false
		childCount := int(modifiers.ChildCount())
		for i := 0; i < childCount; i++ {
			child := modifiers.Child(i)
			if child != nil && child.Type() == "keyword" {
				kw := string(content[child.StartByte():child.EndByte()])
				if kw == "static" {
					isStatic = true
				}
				if kw == "final" {
					isFinal = true
				}
			}
		}
		if isStatic && isFinal {
			return field
		}
	}

	var typeNode *sitter.Node
	var genericTypeNode *sitter.Node
	typeNode = p.findChildByType(fieldNode, "type")
	genericTypeNode = p.findChildByType(fieldNode, "generic_type")

	if typeNode == nil && genericTypeNode == nil {
		typeIdentifierNode := p.findChildByType(fieldNode, "type_identifier")
		if typeIdentifierNode != nil {
			typeName := string(content[typeIdentifierNode.StartByte():typeIdentifierNode.EndByte()])
			field.Type = p.resolveTypeName(typeName)
		}
	} else if genericTypeNode != nil {
		fieldType := p.extractTypeFull(genericTypeNode, content)
		field.IsCollection = fieldType.IsCollection
		field.GenericArgs = fieldType.GenericArgs
		if len(field.GenericArgs) > 0 {
			field.Type = fieldType.SimpleType + "<" + strings.Join(field.GenericArgs, ", ") + ">"
		} else {
			field.Type = fieldType.SimpleType
		}
	} else if typeNode != nil {
		fieldType := p.extractTypeFull(typeNode, content)
		field.Type = fieldType.SimpleType
		field.IsCollection = fieldType.IsCollection
		field.GenericArgs = fieldType.GenericArgs
	}

	declarators := p.findChildByType(fieldNode, "variable_declarators")
	if declarators == nil {
		declarators = p.findChildByType(fieldNode, "variable_declarator")
	}
	if declarators != nil {
		firstDeclarator := p.findChildByType(declarators, "variable_declarator")
		if firstDeclarator != nil {
			varNameNode := p.findChildByType(firstDeclarator, "identifier")
			if varNameNode != nil {
				field.Name = string(content[varNameNode.StartByte():varNameNode.EndByte()])
			}
		} else {
			varNameNode := p.findChildByType(declarators, "identifier")
			if varNameNode != nil {
				field.Name = string(content[varNameNode.StartByte():varNameNode.EndByte()])
			}
		}
	}

	if field.Type != "" && !field.IsCollection && !isPrimitiveOrJavaLang(field.Type) {
		typeDetails := p.resolveTypeDetails(field.Type, depth+1)
		if typeDetails != nil {
			field.TypeDetails = typeDetails
		}
	}

	if field.IsCollection && len(field.GenericArgs) > 0 {
		argType := field.GenericArgs[0]
		if !isPrimitiveOrJavaLang(argType) {
			typeDetails := p.resolveTypeDetails(argType, depth+1)
			if typeDetails != nil {
				field.TypeDetails = &TypeDetails{
					Package:  typeDetails.Package,
					Name:     typeDetails.Name,
					FullName: typeDetails.FullName,
					Fields:   typeDetails.Fields,
				}
			}
		}
	}

	return field
}

func (p *Parser) resolveTypeDetails(typeName string, depth int) *TypeDetails {
	if depth > 40 {
		return nil
	}

	resolved := p.resolveTypeName(typeName)

	// Check imports first
	for _, imp := range p.imports {
		if imp.fullName == resolved && imp.filePath != "" {
			details, err := p.parseSourceFile(imp.filePath)
			if err == nil && details != nil {
				if details.Extends != "" {
					parentDetails := p.resolveTypeDetails(details.Extends, depth+1)
					if parentDetails != nil {
						allFields := make([]Field, len(details.Fields))
						copy(allFields, details.Fields)
						allFields = append(allFields, parentDetails.Fields...)
						details.Fields = allFields
					}
				}
				return details
			}
		}
	}

	// Check if it's a fully qualified name - try to find the source file
	if strings.Contains(resolved, ".") {
		if filePath := p.findSourceFile(resolved); filePath != "" {
			details, err := p.parseSourceFile(filePath)
			if err == nil && details != nil {
				if details.Extends != "" {
					parentDetails := p.resolveTypeDetails(details.Extends, depth+1)
					if parentDetails != nil {
						allFields := make([]Field, len(details.Fields))
						copy(allFields, details.Fields)
						allFields = append(allFields, parentDetails.Fields...)
						details.Fields = allFields
					}
				}
				return details
			}
		}
	}

	// Fallback to local classes (same package as controller)
	simpleName := typeName
	if idx := strings.LastIndex(typeName, "."); idx >= 0 {
		simpleName = typeName[idx+1:]
	}

	if filePath, ok := p.localClasses[simpleName]; ok {
		details, err := p.parseSourceFile(filePath)
		if err == nil && details != nil {
			if details.Extends != "" {
				parentDetails := p.resolveTypeDetails(details.Extends, depth+1)
				if parentDetails != nil {
					allFields := make([]Field, len(details.Fields))
					copy(allFields, details.Fields)
					allFields = append(allFields, parentDetails.Fields...)
					details.Fields = allFields
				}
			}
			return details
		}
	}

	return nil
}

type extractedType struct {
	SimpleType   string
	IsCollection bool
	GenericArgs  []string
}

func (p *Parser) extractTypeFull(typeNode *sitter.Node, content []byte) extractedType {
	result := extractedType{}

	if typeNode.Type() == "generic_type" {
		typeIdent := p.findChildByType(typeNode, "type_identifier")
		if typeIdent != nil {
			baseName := string(content[typeIdent.StartByte():typeIdent.EndByte()])
			result.SimpleType = p.resolveTypeName(baseName)

			if isCollectionType(result.SimpleType) {
				result.IsCollection = true
			}

			typeArgs := p.findChildByType(typeNode, "type_arguments")
			if typeArgs != nil {
				typeList := p.findChildByType(typeArgs, "type_list")
				if typeList != nil {
					childCount := int(typeList.ChildCount())
					for i := 0; i < childCount; i++ {
						child := typeList.Child(i)
						if child != nil && (child.Type() == "type" || child.Type() == "generic_type") {
							argType := p.extractTypeFull(child, content)
							result.GenericArgs = append(result.GenericArgs, argType.SimpleType)
						}
					}
				} else {
					childCount := int(typeArgs.ChildCount())
					for i := 0; i < childCount; i++ {
						child := typeArgs.Child(i)
						if child != nil && child.Type() == "type_identifier" {
							argTypeName := string(content[child.StartByte():child.EndByte()])
							result.GenericArgs = append(result.GenericArgs, p.resolveTypeName(argTypeName))
						}
					}
				}
			}
		}
		return result
	}

	arrayType := p.findChildByType(typeNode, "array_type")
	if arrayType != nil {
		elementType := p.findChildByType(arrayType, "type")
		if elementType != nil {
			baseType := p.extractTypeFull(elementType, content)
			result.SimpleType = baseType.SimpleType + "[]"
			result.IsCollection = true
			result.GenericArgs = baseType.GenericArgs
		}
		return result
	}

	primitiveType := p.findChildByType(typeNode, "primitive_type")
	if primitiveType != nil {
		result.SimpleType = string(content[primitiveType.StartByte():primitiveType.EndByte()])
		return result
	}

	typeIdentifier := p.findChildByType(typeNode, "type_identifier")
	if typeIdentifier != nil {
		typeName := string(content[typeIdentifier.StartByte():typeIdentifier.EndByte()])
		result.SimpleType = p.resolveTypeName(typeName)

		if isCollectionType(result.SimpleType) {
			result.IsCollection = true
		}

		return result
	}

	identifier := p.findChildByType(typeNode, "identifier")
	if identifier != nil {
		typeName := string(content[identifier.StartByte():identifier.EndByte()])
		result.SimpleType = p.resolveTypeName(typeName)
		return result
	}

	return result
}

func isCollectionType(typeName string) bool {
	simpleName := typeName
	if idx := strings.LastIndex(typeName, "."); idx >= 0 {
		simpleName = typeName[idx+1:]
	}
	collections := map[string]bool{
		"List": true, "ArrayList": true, "LinkedList": true,
		"Set": true, "HashSet": true, "TreeSet": true, "LinkedHashSet": true,
		"Map": true, "HashMap": true, "TreeMap": true, "LinkedHashMap": true,
		"Collection": true, "Iterable": true,
	}
	return collections[simpleName]
}

func isPrimitiveOrJavaLang(typeName string) bool {
	simpleName := typeName
	if idx := strings.LastIndex(typeName, "."); idx >= 0 {
		simpleName = typeName[idx+1:]
	}

	javaLangTypes := map[string]bool{
		"String": true, "Integer": true, "Long": true, "Boolean": true,
		"Double": true, "Float": true, "Byte": true, "Short": true,
		"Character": true, "Object": true, "Class": true,
		"BigDecimal": true, "BigInteger": true, "Date": true, "UUID": true,
	}

	return javaLangTypes[simpleName]
}

func (p *Parser) findClassDeclaration(root *sitter.Node) *sitter.Node {
	var classNode *sitter.Node

	iter := sitter.NewIterator(root, sitter.DFSMode)
	for {
		node, err := iter.Next()
		if err != nil || node == nil {
			break
		}
		if node.Type() == "class_declaration" {
			classNode = node
			break
		}
	}

	return classNode
}

func (p *Parser) findPackageDeclaration(root *sitter.Node) *sitter.Node {
	iter := sitter.NewIterator(root, sitter.DFSMode)
	for {
		node, err := iter.Next()
		if err != nil || node == nil {
			break
		}
		if node.Type() == "package_declaration" {
			scopedIdent := p.findChildByType(node, "scoped_identifier")
			if scopedIdent != nil {
				return scopedIdent
			}
		}
	}
	return nil
}

func (p *Parser) findMethodDeclarations(classNode *sitter.Node) []*sitter.Node {
	var methods []*sitter.Node

	classBody := p.findChildByType(classNode, "class_body")
	if classBody == nil {
		return methods
	}

	childCount := int(classBody.ChildCount())
	for i := 0; i < childCount; i++ {
		child := classBody.Child(i)
		if child != nil && child.Type() == "method_declaration" {
			methods = append(methods, child)
		}
	}

	return methods
}

func (p *Parser) findChildByType(node *sitter.Node, expectedType string) *sitter.Node {
	childCount := int(node.ChildCount())
	for i := 0; i < childCount; i++ {
		child := node.Child(i)
		if child != nil && child.Type() == expectedType {
			return child
		}
	}
	return nil
}

func (p *Parser) findLastChildByType(node *sitter.Node, expectedType string) *sitter.Node {
	childCount := int(node.ChildCount())
	var found *sitter.Node
	for i := 0; i < childCount; i++ {
		child := node.Child(i)
		if child != nil && child.Type() == expectedType {
			found = child
		}
	}
	return found
}

func (p *Parser) findAllChildrenByType(node *sitter.Node, expectedType string) []*sitter.Node {
	var nodes []*sitter.Node
	childCount := int(node.ChildCount())
	for i := 0; i < childCount; i++ {
		child := node.Child(i)
		if child != nil && child.Type() == expectedType {
			nodes = append(nodes, child)
		}
	}
	return nodes
}

func (p *Parser) parseClassAnnotations(classNode *sitter.Node, content []byte, result *Result) {
	modifiers := p.findChildByType(classNode, "modifiers")
	if modifiers == nil {
		return
	}

	annotations := p.findAllChildrenByType(modifiers, "annotation")
	for _, ann := range annotations {
		p.parseAnnotation(ann, content, result, true)
	}
}

func (p *Parser) parseAnnotation(node *sitter.Node, content []byte, result *Result, isClassLevel bool) {
	nameNode := p.findChildByType(node, "identifier")
	if nameNode == nil {
		return
	}

	annotationName := string(content[nameNode.StartByte():nameNode.EndByte()])

	switch annotationName {
	case "Path":
		if isClassLevel {
			result.BasePath = p.getAnnotationValueFromAnnotation(node, content)
		}
	case "Produces":
		result.Produces = p.parseAnnotationArray(node, content)
	case "Consumes":
		result.Consumes = p.parseAnnotationArray(node, content)
	}
}

func (p *Parser) getAnnotationValueFromAnnotation(node *sitter.Node, content []byte) string {
	argsList := p.findChildByType(node, "annotation_argument_list")
	if argsList == nil {
		return ""
	}

	childCount := int(argsList.ChildCount())
	for i := 0; i < childCount; i++ {
		child := argsList.Child(i)
		if child == nil {
			continue
		}

		if child.Type() == "field_access" {
			obj := p.findChildByType(child, "object")
			field := p.findChildByType(child, "field")
			if obj != nil && field != nil {
				objName := string(content[obj.StartByte():obj.EndByte()])
				fieldName := string(content[field.StartByte():field.EndByte()])
				return objName + "." + fieldName
			}
		}

		if child.Type() == "string_literal" {
			return p.extractStringValue(child, content)
		}

		if child.Type() == "element_value_pair" {
			keyNode := p.findChildByType(child, "identifier")
			valueNode := p.findChildByType(child, "element_value")
			if keyNode != nil && valueNode != nil {
				keyName := string(content[keyNode.StartByte():keyNode.EndByte()])
				if keyName == "value" || keyName == "path" {
					return p.extractStringValue(valueNode, content)
				}
			}
		}
	}

	return ""
}

func (p *Parser) parseAnnotationArray(node *sitter.Node, content []byte) []string {
	var values []string

	argsList := p.findChildByType(node, "annotation_argument_list")
	if argsList == nil {
		return values
	}

	childCount := int(argsList.ChildCount())
	for i := 0; i < childCount; i++ {
		child := argsList.Child(i)
		if child == nil {
			continue
		}

		if child.Type() == "field_access" {
			var objName, fieldName string
			obj := p.findChildByType(child, "object")
			field := p.findChildByType(child, "field")
			if obj != nil && field != nil {
				objName = string(content[obj.StartByte():obj.EndByte()])
				fieldName = string(content[field.StartByte():field.EndByte()])
			} else {
				id1 := p.findChildByType(child, "identifier")
				if id1 != nil {
					objName = string(content[id1.StartByte():id1.EndByte()])
					for j := 0; j < int(child.ChildCount()); j++ {
						grandchild := child.Child(j)
						if grandchild != nil && grandchild.Type() == "identifier" && j > 0 {
							fieldName = string(content[grandchild.StartByte():grandchild.EndByte()])
							break
						}
					}
				}
			}
			if objName != "" && fieldName != "" {
				values = append(values, objName+"."+fieldName)
			}
		}

		if child.Type() == "string_literal" {
			val := p.extractStringValue(child, content)
			if val != "" {
				values = append(values, val)
			}
		}

		if child.Type() == "element_value_pair" {
			valueNode := p.findChildByType(child, "element_value")
			if valueNode != nil {
				val := p.extractStringValue(valueNode, content)
				if val != "" {
					values = append(values, val)
				}
			}
		}
	}

	return values
}

func (p *Parser) extractStringValue(node *sitter.Node, content []byte) string {
	if node == nil {
		return ""
	}

	if node.Type() == "string_literal" {
		frag := p.findChildByType(node, "string_fragment")
		if frag != nil {
			return string(content[frag.StartByte():frag.EndByte()])
		}
		text := string(content[node.StartByte():node.EndByte()])
		if len(text) >= 2 {
			return text[1 : len(text)-1]
		}
	}

	childCount := int(node.ChildCount())
	if childCount > 0 {
		for i := 0; i < childCount; i++ {
			child := node.Child(i)
			if child != nil && child.Type() == "string_literal" {
				return p.extractStringValue(child, content)
			}
		}
	}

	return ""
}

func (p *Parser) parseMethod(methodNode *sitter.Node, content []byte, result *Result) *Endpoint {
	modifiers := p.findChildByType(methodNode, "modifiers")
	if modifiers == nil {
		return nil
	}

	var httpMethod string
	var methodPath string
	var methodProduces []string
	var methodConsumes []string

	markerAnnotations := p.findAllChildrenByType(modifiers, "marker_annotation")
	annotations := p.findAllChildrenByType(modifiers, "annotation")

	for _, ma := range markerAnnotations {
		nameNode := p.findChildByType(ma, "identifier")
		if nameNode == nil {
			continue
		}
		annotationName := string(content[nameNode.StartByte():nameNode.EndByte()])

		if isHTTPMethod(annotationName) {
			httpMethod = annotationName
		}
	}

	for _, ann := range annotations {
		nameNode := p.findChildByType(ann, "identifier")
		if nameNode == nil {
			continue
		}
		annotationName := string(content[nameNode.StartByte():nameNode.EndByte()])

		if isHTTPMethod(annotationName) {
			httpMethod = annotationName
			methodPath = p.getAnnotationValueFromAnnotation(ann, content)
		} else if annotationName == "Path" {
			methodPath = p.getAnnotationValueFromAnnotation(ann, content)
		} else if annotationName == "Produces" {
			methodProduces = p.parseAnnotationArray(ann, content)
		} else if annotationName == "Consumes" {
			methodConsumes = p.parseAnnotationArray(ann, content)
		}
	}

	if httpMethod == "" {
		return nil
	}

	var returnType string

	typeNode := p.findChildByType(methodNode, "type")
	if typeNode != nil {
		returnType = p.extractType(typeNode, content)
	}

	if returnType == "" {
		typeIdentifierNode := p.findChildByType(methodNode, "type_identifier")
		if typeIdentifierNode != nil {
			returnType = p.resolveTypeName(string(content[typeIdentifierNode.StartByte():typeIdentifierNode.EndByte()]))
		}
	}

	if returnType == "" {
		genericTypeNode := p.findChildByType(methodNode, "generic_type")
		if genericTypeNode != nil {
			returnType = p.extractGenericType(genericTypeNode, content)
		}
	}

	voidTypeNode := p.findChildByType(methodNode, "void_type")
	if voidTypeNode != nil {
		returnType = "void"
	}

	nameNode := p.findChildByType(methodNode, "identifier")
	var handler string
	if nameNode != nil {
		handler = string(content[nameNode.StartByte():nameNode.EndByte()])
	}

	fullPath := result.BasePath + methodPath

	produces := methodProduces
	if len(produces) == 0 {
		produces = result.Produces
	}

	consumes := methodConsumes
	if len(consumes) == 0 {
		consumes = result.Consumes
	}

	actualReturnTypes := []string{}
	if isResponseType(returnType) {
		bodyReturnTypes := p.parseMethodBody(methodNode, content, 0)
		if len(bodyReturnTypes) > 0 {
			actualReturnTypes = bodyReturnTypes
		} else {
			actualReturnTypes = []string{returnType}
		}
	} else {
		actualReturnTypes = []string{returnType}
	}

	endpoint := &Endpoint{
		Method:      httpMethod,
		Path:        fullPath,
		ReturnType:  returnType,
		ReturnTypes: actualReturnTypes,
		Handler:     handler,
		Consumes:    consumes,
		Produces:    produces,
	}

	if len(actualReturnTypes) > 0 {
		returnTypeDetails := make([]*TypeDetails, len(actualReturnTypes))
		var primaryTypeDetails *TypeDetails

		for i, returnType := range actualReturnTypes {
			if returnType == "" || returnType == "void" {
				continue
			}

			// Check if it's a generic type like "List<User>" or "java.util.List<User>"
			if strings.Contains(returnType, "<") {
				// Extract generic type argument
				startIdx := strings.Index(returnType, "<")
				endIdx := strings.LastIndex(returnType, ">")
				if startIdx > 0 && endIdx > startIdx {
					genericArg := returnType[startIdx+1 : endIdx]
					// Resolve the generic argument type
					simpleName := genericArg
					if idx := strings.LastIndex(genericArg, "."); idx >= 0 {
						simpleName = genericArg[idx+1:]
					}
					if !isPrimitiveOrJavaLang(genericArg) {
						typeDetails := p.resolveTypeDetails(simpleName, 0)
						if typeDetails != nil {
							returnTypeDetails[i] = typeDetails
							if primaryTypeDetails == nil {
								primaryTypeDetails = typeDetails
							}
						}
					}
				}
			} else if !isPrimitiveOrJavaLang(returnType) {
				simpleName := returnType
				if idx := strings.LastIndex(returnType, "."); idx >= 0 {
					simpleName = returnType[idx+1:]
				}
				typeDetails := p.resolveTypeDetails(simpleName, 0)
				if typeDetails != nil {
					returnTypeDetails[i] = typeDetails
					if primaryTypeDetails == nil {
						primaryTypeDetails = typeDetails
					}
				}
			}
		}

		endpoint.ReturnTypeDetails = returnTypeDetails
		if primaryTypeDetails != nil {
			endpoint.TypeDetails = primaryTypeDetails
		}
	}

	return endpoint
}

func (p *Parser) extractType(typeNode *sitter.Node, content []byte) string {
	genericType := p.findChildByType(typeNode, "generic_type")
	if genericType != nil {
		return p.extractGenericType(genericType, content)
	}

	arrayType := p.findChildByType(typeNode, "array_type")
	if arrayType != nil {
		elementType := p.findChildByType(arrayType, "type")
		if elementType != nil {
			baseType := p.extractType(elementType, content)
			return baseType + "[]"
		}
	}

	primitiveType := p.findChildByType(typeNode, "primitive_type")
	if primitiveType != nil {
		return string(content[primitiveType.StartByte():primitiveType.EndByte()])
	}

	typeIdentifier := p.findChildByType(typeNode, "type_identifier")
	if typeIdentifier != nil {
		typeName := string(content[typeIdentifier.StartByte():typeIdentifier.EndByte()])
		return p.resolveTypeName(typeName)
	}

	identifier := p.findChildByType(typeNode, "identifier")
	if identifier != nil {
		typeName := string(content[identifier.StartByte():identifier.EndByte()])
		return p.resolveTypeName(typeName)
	}

	return ""
}

func (p *Parser) extractGenericType(node *sitter.Node, content []byte) string {
	typeNode := p.findChildByType(node, "type")
	if typeNode == nil {
		typeNode = p.findChildByType(node, "type_identifier")
	}

	var baseType string
	if typeNode != nil {
		if typeNode.Type() == "type_identifier" {
			baseType = string(content[typeNode.StartByte():typeNode.EndByte()])
		} else {
			baseType = p.extractType(typeNode, content)
		}
	}

	typeArguments := p.findChildByType(node, "type_arguments")
	if typeArguments == nil {
		return p.resolveTypeName(baseType)
	}

	var typeArgs []string

	// Try to find type_list first
	typeArgList := p.findChildByType(typeArguments, "type_list")
	if typeArgList != nil {
		childCount := int(typeArgList.ChildCount())
		for i := 0; i < childCount; i++ {
			child := typeArgList.Child(i)
			if child != nil && (child.Type() == "type" || child.Type() == "generic_type" || child.Type() == "type_identifier") {
				var argType string
				if child.Type() == "type_identifier" {
					argType = string(content[child.StartByte():child.EndByte()])
				} else if child.Type() == "type" {
					argType = p.extractType(child, content)
				} else if child.Type() == "generic_type" {
					argType = p.extractGenericType(child, content)
				}
				if argType != "" {
					typeArgs = append(typeArgs, argType)
				}
			}
		}
	} else {
		// No type_list, look for type_identifier directly in type_arguments
		childCount := int(typeArguments.ChildCount())
		for i := 0; i < childCount; i++ {
			child := typeArguments.Child(i)
			if child != nil && child.Type() == "type_identifier" {
				argType := string(content[child.StartByte():child.EndByte()])
				if argType != "" {
					typeArgs = append(typeArgs, argType)
				}
			}
		}
	}

	resolvedBase := p.resolveTypeName(baseType)
	if len(typeArgs) > 0 {
		return resolvedBase + "<" + strings.Join(typeArgs, ", ") + ">"
	}

	return resolvedBase
}

func (p *Parser) resolveTypeName(typeName string) string {
	if typeName == "" {
		return typeName
	}

	if _, ok := p.primitives[typeName]; ok {
		if _, ok := p.javaLang[typeName]; ok {
			return "java.lang." + typeName
		}
		return typeName
	}

	if strings.Contains(typeName, ".") {
		return typeName
	}

	for _, imp := range p.imports {
		if imp.simpleName == typeName {
			return imp.fullName
		}
	}

	if filePath, ok := p.localClasses[typeName]; ok {
		details, err := p.parseSourceFile(filePath)
		if err == nil && details != nil {
			return details.FullName
		}
	}

	if isCollectionType(typeName) {
		return "java.util." + typeName
	}

	if p.currentPkg != "" {
		return p.currentPkg + "." + typeName
	}

	return typeName
}

func (p *Parser) parseMethodBody(methodNode *sitter.Node, content []byte, depth int) []string {
	if depth > 20 {
		return nil
	}

	var returnTypes []string

	blockNode := p.findChildByType(methodNode, "block")
	if blockNode == nil {
		return nil
	}

	returnStatements := p.findReturnStatements(blockNode, content)
	for _, returnStmt := range returnStatements {
		returnType := p.extractReturnExpression(returnStmt, methodNode, content, depth)
		if returnType != "" {
			returnTypes = append(returnTypes, returnType)
		}
	}

	return deduplicateStrings(returnTypes)
}

func (p *Parser) findReturnStatements(blockNode *sitter.Node, content []byte) []*sitter.Node {
	var returnStmts []*sitter.Node

	iter := sitter.NewIterator(blockNode, sitter.DFSMode)
	for {
		node, err := iter.Next()
		if err != nil || node == nil {
			break
		}
		if node.Type() == "return_statement" {
			returnStmts = append(returnStmts, node)
		}
	}

	return returnStmts
}

func (p *Parser) extractReturnExpression(returnNode *sitter.Node, methodNode *sitter.Node, content []byte, depth int) string {
	childCount := int(returnNode.ChildCount())
	if childCount < 2 {
		return ""
	}

	exprNode := returnNode.Child(1)
	if exprNode == nil {
		return ""
	}

	if exprNode.Type() == "null_literal" {
		return ""
	}

	if exprNode.Type() == "method_invocation" {
		return p.analyzeMethodInvocation(exprNode, methodNode, content, depth)
	}

	if exprNode.Type() == "object_creation_expression" {
		return p.analyzeObjectCreation(exprNode, content, depth)
	}

	if exprNode.Type() == "identifier" {
		varName := string(content[exprNode.StartByte():exprNode.EndByte()])
		return p.resolveVariableType(varName, methodNode, content, depth)
	}

	if exprNode.Type() == "binary_expression" || exprNode.Type() == "conditional_expression" {
		return p.analyzeExpression(exprNode, content, depth)
	}

	return ""
}

func (p *Parser) analyzeMethodInvocation(node *sitter.Node, methodNode *sitter.Node, content []byte, depth int) string {
	methodNameNode := p.findChildByType(node, "identifier")
	if methodNameNode != nil {
		methodName := string(content[methodNameNode.StartByte():methodNameNode.EndByte()])

		objectNode := p.findChildByType(node, "object")
		var objectName string
		if objectNode != nil {
			objectName = string(content[objectNode.StartByte():objectNode.EndByte()])
		}

		isResponse := objectName == "Response" || methodName == "ok" || methodName == "noContent" ||
			methodName == "created" || methodName == "status" || methodName == "accepted" ||
			methodName == "notAcceptable" || methodName == "notModified" || methodName == "seeOther" ||
			methodName == "temporaryRedirect" || methodName == "fromResponse" || methodName == "build"

		if isResponse {
			return p.analyzeResponseBuilder(node, methodNode, content, depth)
		}

		if objectName != "" {
			returnType := p.resolveMethodCall(objectName, methodName, nil, node, content, depth)
			if returnType != "" {
				return returnType
			}
		}

		returnType := p.resolveMethodCall("", methodName, nil, node, content, depth)
		if returnType != "" {
			return returnType
		}
	}

	return ""
}

func (p *Parser) analyzeResponseBuilder(node *sitter.Node, methodNode *sitter.Node, content []byte, depth int) string {
	chain := p.getMethodChain(node, content)

	for i, call := range chain {
		methodName := call.method
		args := call.args

		switch methodName {
		case "noContent", "ok", "accepted", "notModified":
			if i == len(chain)-1 {
				return "void"
			}
		case "created":
			if i == len(chain)-1 {
				return ""
			}
		case "entity":
			if len(args) > 0 {
				return p.resolveVariableType(args[0], methodNode, content, depth)
			}
		case "build":
			if i > 0 {
				prevCall := chain[i-1]
				if prevCall.method == "entity" && len(prevCall.args) > 0 {
					return p.resolveVariableType(prevCall.args[0], methodNode, content, depth)
				}
				if prevCall.method == "noContent" {
					return "void"
				}
				if prevCall.method == "ok" || prevCall.method == "accepted" ||
					prevCall.method == "created" || prevCall.method == "notModified" {
					if len(prevCall.args) > 0 {
						return p.resolveVariableType(prevCall.args[0], methodNode, content, depth)
					}
					return "void"
				}
				// Response.status(code).build() without entity() -> void
				if prevCall.method == "status" {
					return "void"
				}
			}
		case "status":
			if len(args) > 1 {
				entityIdx := p.findArgIndex(args, "entity")
				if entityIdx > 0 && entityIdx < len(args)-1 {
					return p.resolveVariableType(args[entityIdx+1], methodNode, content, depth)
				}
			}
		}
	}

	return ""
}

type methodCall struct {
	method string
	args   []string
}

func (p *Parser) getMethodChain(node *sitter.Node, content []byte) []methodCall {
	var chain []methodCall

	current := node
	for current != nil {
		// For method_invocation, find the LAST identifier (the method name, not the object)
		var methodNameNode *sitter.Node
		if current.Type() == "method_invocation" {
			methodNameNode = p.findLastChildByType(current, "identifier")
		} else {
			methodNameNode = p.findChildByType(current, "identifier")
		}
		if methodNameNode == nil {
			break
		}
		methodName := string(content[methodNameNode.StartByte():methodNameNode.EndByte()])

		args := p.extractArguments(current, content)

		chain = append([]methodCall{{method: methodName, args: args}}, chain...)

		// Try to find the object/ receiver of this method call
		// It could be a nested method_invocation (for chained calls like a.b().c())
		// or it could be an identifier (like Response.ok())
		var prevNode *sitter.Node

		// Look for method_invocation as first child (chained call pattern)
		childCount := int(current.ChildCount())
		for i := 0; i < childCount; i++ {
			child := current.Child(i)
			if child != nil && child.Type() == "method_invocation" {
				prevNode = child
				break
			}
		}

		// Also check for "object" child (field access pattern)
		if prevNode == nil {
			objectNode := p.findChildByType(current, "object")
			if objectNode != nil {
				prevNode = objectNode
			}
		}

		if prevNode == nil {
			break
		}

		if prevNode.Type() == "method_invocation" {
			current = prevNode
		} else if prevNode.Type() == "identifier" {
			break
		} else {
			break
		}
	}

	return chain
}

func (p *Parser) extractArguments(node *sitter.Node, content []byte) []string {
	var args []string

	argList := p.findChildByType(node, "argument_list")
	if argList == nil {
		return args
	}

	childCount := int(argList.ChildCount())
	for i := 1; i < childCount-1; i++ {
		child := argList.Child(i)
		if child != nil {
			args = append(args, string(content[child.StartByte():child.EndByte()]))
		}
	}

	return args
}

func (p *Parser) findArgIndex(args []string, target string) int {
	for i, arg := range args {
		if arg == target {
			return i
		}
	}
	return -1
}

func (p *Parser) extractTypeFromExpression(expr string, content []byte, depth int) string {
	// First try to resolve as a type name
	returnType := p.resolveTypeName(expr)
	if returnType != expr && !strings.Contains(returnType, ".") == false {
		return returnType
	}

	// If it's just the expression itself, it might be a variable
	// We need to look it up in the current context
	// For now, return the expression as-is, the caller can handle it
	return expr
}

func (p *Parser) analyzeObjectCreation(node *sitter.Node, content []byte, depth int) string {
	typeNode := p.findChildByType(node, "type")
	if typeNode != nil {
		return p.extractType(typeNode, content)
	}

	typeIdentifier := p.findChildByType(node, "type_identifier")
	if typeIdentifier != nil {
		typeName := string(content[typeIdentifier.StartByte():typeIdentifier.EndByte()])
		return p.resolveTypeName(typeName)
	}

	return ""
}

func (p *Parser) resolveVariableType(varName string, methodNode *sitter.Node, content []byte, depth int) string {
	// If methodNode is provided, get parameters and local variables from that specific method
	if methodNode != nil {
		params := p.collectMethodParametersFromNode(methodNode, content)
		paramType, ok := params[varName]
		if ok {
			return paramType
		}

		// Also check local variables
		locals := p.collectLocalVariables(methodNode, content)
		localType, ok := locals[varName]
		if ok {
			return localType
		}
	}

	// Also check class fields
	classNode := p.findClassDeclaration(rootFromNode(methodNode))
	if classNode == nil {
		return ""
	}

	fields := p.collectClassFields(classNode, content)
	fieldType, ok := fields[varName]
	if ok {
		if strings.HasSuffix(fieldType, ">") && !strings.Contains(fieldType, ".") {
			fieldType = "java.util." + fieldType
		}
		if !strings.Contains(fieldType, ".") {
			fieldType = p.resolveTypeName(fieldType)
		}
		return fieldType
	}

	return ""
}

func (p *Parser) collectMethodParametersFromNode(methodNode *sitter.Node, content []byte) map[string]string {
	params := make(map[string]string)

	paramsNode := p.findChildByType(methodNode, "formal_parameters")
	if paramsNode == nil {
		return params
	}

	childCount := int(paramsNode.ChildCount())
	for i := 0; i < childCount; i++ {
		child := paramsNode.Child(i)
		if child != nil && child.Type() == "formal_parameter" {
			// Try "type" first, then "type_identifier"
			paramTypeNode := p.findChildByType(child, "type")
			if paramTypeNode == nil {
				paramTypeNode = p.findChildByType(child, "type_identifier")
			}
			paramNameNode := p.findChildByType(child, "identifier")

			if paramTypeNode != nil && paramNameNode != nil {
				var paramType string
				if paramTypeNode.Type() == "type_identifier" {
					paramType = string(content[paramTypeNode.StartByte():paramTypeNode.EndByte()])
					paramType = p.resolveTypeName(paramType)
				} else {
					paramType = p.extractType(paramTypeNode, content)
				}
				paramName := string(content[paramNameNode.StartByte():paramNameNode.EndByte()])
				params[paramName] = paramType
			}
		}
	}

	return params
}

func (p *Parser) collectLocalVariables(methodNode *sitter.Node, content []byte) map[string]string {
	locals := make(map[string]string)

	blockNode := p.findChildByType(methodNode, "block")
	if blockNode == nil {
		return locals
	}

	// Find all local variable declarations
	iter := sitter.NewIterator(blockNode, sitter.DFSMode)
	for {
		node, err := iter.Next()
		if err != nil || node == nil {
			break
		}
		if node.Type() == "local_variable_declaration" {
			// Get the type - check for type, type_identifier, or generic_type
			var typeNode *sitter.Node
			childCount := int(node.ChildCount())
			for i := 0; i < childCount; i++ {
				child := node.Child(i)
				if child != nil && (child.Type() == "type" || child.Type() == "type_identifier" || child.Type() == "generic_type") {
					typeNode = child
					break
				}
			}

			// Get declarators
			declarators := p.findChildByType(node, "variable_declarators")
			if declarators == nil {
				declarators = p.findChildByType(node, "variable_declarator")
			}

			if typeNode != nil && declarators != nil {
				var varType string
				if typeNode.Type() == "type_identifier" {
					varType = string(content[typeNode.StartByte():typeNode.EndByte()])
					varType = p.resolveTypeName(varType)
				} else if typeNode.Type() == "generic_type" {
					varType = p.extractGenericType(typeNode, content)
				} else {
					varType = p.extractType(typeNode, content)
				}

				// Get variable name
				firstDecl := p.findChildByType(declarators, "variable_declarator")
				if firstDecl == nil {
					firstDecl = declarators
				}
				varNameNode := p.findChildByType(firstDecl, "identifier")
				if varNameNode != nil {
					varName := string(content[varNameNode.StartByte():varNameNode.EndByte()])
					locals[varName] = varType
				}
			}
		}
	}

	return locals
}

func (p *Parser) collectMethodParameters(classNode *sitter.Node, content []byte) map[string]string {
	params := make(map[string]string)

	methods := p.findMethodDeclarations(classNode)
	for _, method := range methods {
		paramsNode := p.findChildByType(method, "formal_parameters")
		if paramsNode == nil {
			continue
		}

		childCount := int(paramsNode.ChildCount())
		for i := 0; i < childCount; i++ {
			child := paramsNode.Child(i)
			if child != nil && child.Type() == "formal_parameter" {
				paramTypeNode := p.findChildByType(child, "type")
				paramNameNode := p.findChildByType(child, "identifier")

				if paramTypeNode != nil && paramNameNode != nil {
					paramType := p.extractType(paramTypeNode, content)
					paramName := string(content[paramNameNode.StartByte():paramNameNode.EndByte()])
					params[paramName] = paramType
				}
			}
		}
	}

	return params
}

func (p *Parser) collectClassFields(classNode *sitter.Node, content []byte) map[string]string {
	fields := make(map[string]string)

	classBody := p.findChildByType(classNode, "class_body")
	if classBody == nil {
		return fields
	}

	childCount := int(classBody.ChildCount())
	for i := 0; i < childCount; i++ {
		child := classBody.Child(i)
		if child != nil && child.Type() == "field_declaration" {
			fieldTypeNode := p.findChildByType(child, "type")
			if fieldTypeNode == nil {
				typeIdentifierNode := p.findChildByType(child, "type_identifier")
				if typeIdentifierNode != nil {
					fieldType := string(content[fieldTypeNode.StartByte():fieldTypeNode.EndByte()])
					declarators := p.findChildByType(child, "variable_declarators")
					if declarators == nil {
						declarators = p.findChildByType(child, "variable_declarator")
					}
					if declarators != nil {
						varNameNode := p.findChildByType(declarators, "identifier")
						if varNameNode != nil {
							varName := string(content[varNameNode.StartByte():varNameNode.EndByte()])
							fields[varName] = fieldType
						}
					}
				}
				continue
			}

			fieldType := p.extractType(fieldTypeNode, content)
			if fieldType == "" {
				typeIdentifierNode := p.findChildByType(child, "type_identifier")
				if typeIdentifierNode != nil {
					fieldType = string(content[typeIdentifierNode.StartByte():typeIdentifierNode.EndByte()])
				}
			}

			declarators := p.findChildByType(child, "variable_declarators")
			if declarators == nil {
				declarators = p.findChildByType(child, "variable_declarator")
			}
			if declarators != nil {
				firstDecl := p.findChildByType(declarators, "variable_declarator")
				if firstDecl == nil {
					varNameNode := p.findChildByType(declarators, "identifier")
					if varNameNode != nil {
						varName := string(content[varNameNode.StartByte():varNameNode.EndByte()])
						fields[varName] = fieldType
					}
				} else {
					varNameNode := p.findChildByType(firstDecl, "identifier")
					if varNameNode != nil {
						varName := string(content[varNameNode.StartByte():varNameNode.EndByte()])
						fields[varName] = fieldType
					}
				}
			}
		}
	}

	return fields
}

func (p *Parser) resolveMethodCall(objectName, methodName string, args []string, contextNode *sitter.Node, content []byte, depth int) string {
	if depth > 20 {
		return ""
	}

	classNode := p.findClassDeclaration(rootFromNode(contextNode))
	if classNode == nil {
		return ""
	}

	methods := p.findMethodDeclarations(classNode)
	for _, method := range methods {
		nameNode := p.findChildByType(method, "identifier")
		if nameNode == nil {
			continue
		}

		currentMethodName := string(content[nameNode.StartByte():nameNode.EndByte()])
		if currentMethodName != methodName {
			continue
		}

		returnType := p.extractMethodReturnType(method, content)
		if returnType == "" {
			typeIdentifierNode := p.findChildByType(method, "type_identifier")
			if typeIdentifierNode != nil {
				returnType = p.resolveTypeName(string(content[typeIdentifierNode.StartByte():typeIdentifierNode.EndByte()]))
			}
		}

		if returnType == "" {
			voidTypeNode := p.findChildByType(method, "void_type")
			if voidTypeNode != nil {
				returnType = "void"
			}
		}

		if returnType == "jakarta.ws.rs.core.Response" || returnType == "javax.ws.rs.core.Response" || returnType == "Response" {
			bodyReturnTypes := p.parseMethodBody(method, content, depth+1)
			if len(bodyReturnTypes) > 0 {
				return bodyReturnTypes[0]
			}
			return "jakarta.ws.rs.core.Response"
		}

		return returnType
	}

	return ""
}

func (p *Parser) extractMethodReturnType(methodNode *sitter.Node, content []byte) string {
	typeNode := p.findChildByType(methodNode, "type")
	if typeNode != nil {
		return p.extractType(typeNode, content)
	}

	typeIdentifierNode := p.findChildByType(methodNode, "type_identifier")
	if typeIdentifierNode != nil {
		return p.resolveTypeName(string(content[typeIdentifierNode.StartByte():typeIdentifierNode.EndByte()]))
	}

	genericTypeNode := p.findChildByType(methodNode, "generic_type")
	if genericTypeNode != nil {
		return p.extractGenericType(genericTypeNode, content)
	}

	voidTypeNode := p.findChildByType(methodNode, "void_type")
	if voidTypeNode != nil {
		return "void"
	}

	return ""
}

func (p *Parser) analyzeExpression(exprNode *sitter.Node, content []byte, depth int) string {
	var types []string

	iter := sitter.NewIterator(exprNode, sitter.DFSMode)
	for {
		node, err := iter.Next()
		if err != nil || node == nil {
			break
		}

		if node.Type() == "identifier" {
			varName := string(content[node.StartByte():node.EndByte()])
			varType := p.resolveVariableType(varName, exprNode, content, depth)
			if varType != "" {
				types = append(types, varType)
			}
		}

		if node.Type() == "method_invocation" {
			// Pass nil for methodNode since we don't have the context here
			methodType := p.analyzeMethodInvocation(node, nil, content, depth)
			if methodType != "" {
				types = append(types, methodType)
			}
		}
	}

	if len(types) > 0 {
		return types[0]
	}

	return ""
}

func deduplicateStrings(strs []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, s := range strs {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

func isResponseType(typeName string) bool {
	if typeName == "" {
		return false
	}

	simpleName := typeName
	if idx := strings.LastIndex(typeName, "."); idx >= 0 {
		simpleName = typeName[idx+1:]
	}

	return simpleName == "Response" || strings.Contains(typeName, "jakarta.ws.rs.core.Response") || strings.Contains(typeName, "javax.ws.rs.core.Response")
}

func isHTTPMethod(annotation string) bool {
	switch annotation {
	case "GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS":
		return true
	default:
		return false
	}
}

func (p *Parser) Close() {
	if p.parser != nil {
		p.parser.Close()
	}
}

func rootFromNode(node *sitter.Node) *sitter.Node {
	current := node
	for current.Parent() != nil {
		current = current.Parent()
	}
	return current
}
