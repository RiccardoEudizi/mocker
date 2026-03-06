package main

import (
	"fmt"
	"os"

	sitter "github.com/smacker/go-tree-sitter"
	java "github.com/tree-sitter/tree-sitter-java/bindings/go"
)

func main() {
	content, _ := os.ReadFile("test/com/example/UserController.java")

	lang := sitter.NewLanguage(java.Language())
	p := sitter.NewParser()
	p.SetLanguage(lang)

	tree := p.Parse(nil, content)
	defer tree.Close()

	root := tree.RootNode()

	iter := sitter.NewIterator(root, sitter.DFSMode)
	for {
		node, err := iter.Next()
		if err != nil || node == nil {
			break
		}
		if node.Type() == "method_declaration" {
			nameNode := findChildByType(node, "identifier")
			if nameNode != nil {
				name := string(content[nameNode.StartByte():nameNode.EndByte()])
				if name == "getUserOrders" {
					fmt.Printf("Method: %s\n", name)

					// Find local variable declarations
					blockNode := findChildByType(node, "block")
					if blockNode != nil {
						iter2 := sitter.NewIterator(blockNode, sitter.DFSMode)
						for {
							child, err2 := iter2.Next()
							if err2 != nil || child == nil {
								break
							}
							if child.Type() == "local_variable_declaration" {
								fmt.Printf("Found local variable declaration: %s\n", string(content[child.StartByte():child.EndByte()]))
								printNode(child, content, 1)
							}
						}
					}
				}
			}
		}
	}
}

func findChildByType(node *sitter.Node, expectedType string) *sitter.Node {
	childCount := int(node.ChildCount())
	for i := 0; i < childCount; i++ {
		child := node.Child(i)
		if child != nil && child.Type() == expectedType {
			return child
		}
	}
	return nil
}

func printNode(n *sitter.Node, content []byte, indent int) {
	prefix := ""
	for i := 0; i < indent; i++ {
		prefix += "  "
	}

	if n.ChildCount() == 0 {
		fmt.Printf("%s%s = %s\n", prefix, n.Type(), string(content[n.StartByte():n.EndByte()]))
	} else {
		fmt.Printf("%s%s:\n", prefix, n.Type())
		for i := 0; i < int(n.ChildCount()); i++ {
			child := n.Child(i)
			if child != nil {
				printNode(child, content, indent+1)
			}
		}
	}
}
