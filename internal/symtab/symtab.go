package symtab

// Location identifies the source position of a symbol.
type Location struct {
	File string `json:"file"`
	Line int    `json:"line"`
}

// FieldInfo describes a single field of a struct type.
type FieldInfo struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Tag     string `json:"tag,omitempty"`
	Comment string `json:"comment,omitempty"`
}

// FuncInfo describes a function or method.
type FuncInfo struct {
	Name      string   `json:"name"`
	Package   string   `json:"package"`
	Receiver  string   `json:"receiver,omitempty"`
	Signature string   `json:"signature"`
	Doc       string   `json:"doc,omitempty"`
	Body      string   `json:"body,omitempty"`
	Location  Location `json:"location"`
}

// TypeKind classifies a named type.
type TypeKind string

const (
	TypeKindStruct    TypeKind = "struct"
	TypeKindInterface TypeKind = "interface"
	TypeKindAlias     TypeKind = "alias"
	TypeKindOther     TypeKind = "other"
)

// TypeInfo describes a named type (struct, interface, or other).
type TypeInfo struct {
	Name     string      `json:"name"`
	Package  string      `json:"package"`
	Kind     TypeKind    `json:"kind"`
	Fields   []FieldInfo `json:"fields,omitempty"`  // struct fields
	Methods  []FuncInfo  `json:"methods,omitempty"` // declared methods
	Embeds   []string    `json:"embeds,omitempty"`  // embedded type names
	Doc      string      `json:"doc,omitempty"`
	Location Location    `json:"location"`
}

// VarInfo describes a package-level variable or constant.
type VarInfo struct {
	Name     string   `json:"name"`
	Package  string   `json:"package"`
	Type     string   `json:"type"`
	IsConst  bool     `json:"is_const"`
	Doc      string   `json:"doc,omitempty"`
	Location Location `json:"location"`
}

// PackageInfo holds all indexed symbols for a single Go package.
type PackageInfo struct {
	ImportPath string     `json:"import_path"`
	Name       string     `json:"name"`
	Dir        string     `json:"dir"`
	Files      []string   `json:"files"`
	Funcs      []FuncInfo `json:"funcs"`
	Types      []TypeInfo `json:"types"`
	Vars       []VarInfo  `json:"vars"`
}

// SymbolKind classifies a symbol returned by FindSymbol.
type SymbolKind string

const (
	SymbolKindFunc   SymbolKind = "func"
	SymbolKindMethod SymbolKind = "method"
	SymbolKindType   SymbolKind = "type"
	SymbolKindVar    SymbolKind = "var"
	SymbolKindConst  SymbolKind = "const"
)

// SymbolRef is a lightweight reference returned by cross-package symbol search.
type SymbolRef struct {
	Name      string     `json:"name"`
	Package   string     `json:"package"`
	Kind      SymbolKind `json:"kind"`
	Receiver  string     `json:"receiver,omitempty"`
	Signature string     `json:"signature,omitempty"`
	Location  Location   `json:"location"`
}
