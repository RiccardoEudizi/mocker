package parser

import "encoding/json"

type Field struct {
	Name         string       `json:"name"`
	Type         string       `json:"type"`
	TypeDetails  *TypeDetails `json:"typeDetails,omitempty"`
	IsCollection bool         `json:"isCollection"`
	GenericArgs  []string     `json:"genericArgs,omitempty"`
}

type TypeDetails struct {
	Package      string   `json:"package"`
	Name         string   `json:"name"`
	FullName     string   `json:"fullName"`
	IsCollection bool     `json:"isCollection,omitempty"`
	GenericArgs  []string `json:"genericArgs,omitempty"`
	Fields       []Field  `json:"fields,omitempty"`
	Extends      string   `json:"extends,omitempty"`
	Implements   []string `json:"implements,omitempty"`
}

type Endpoint struct {
	Method            string         `json:"method"`
	Path              string         `json:"path"`
	ReturnType        string         `json:"returnType"`
	ReturnTypes       []string       `json:"returnTypes,omitempty"`
	TypeDetails       *TypeDetails   `json:"typeDetails,omitempty"`
	ReturnTypeDetails []*TypeDetails `json:"returnTypeDetails,omitempty"`
	Handler           string         `json:"handler"`
	Consumes          []string       `json:"consumes"`
	Produces          []string       `json:"produces"`
}

type Result struct {
	Filename  string     `json:"filename"`
	BasePath  string     `json:"basePath"`
	Produces  []string   `json:"produces"`
	Consumes  []string   `json:"consumes"`
	Endpoints []Endpoint `json:"endpoints"`
}

func (r *Result) ToJSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}
