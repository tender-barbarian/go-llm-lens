package indexer

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"strings"

	"github.com/tender-barbarian/go-llm-lens/internal/symtab"
	"golang.org/x/tools/go/packages"
)

// Indexer holds the fully type-checked in-memory index of a Go codebase.
type Indexer struct {
	root     string
	fset     *token.FileSet
	PkgInfos map[string]*symtab.PackageInfo
	TypePkgs map[string]*types.Package // all loaded packages, including deps, for Implements checks
}

// New creates an Indexer rooted at rootPath. Call Index to load and scan packages.
func New(rootPath string) (*Indexer, error) {
	absRoot, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, fmt.Errorf("resolving root path: %w", err)
	}
	return &Indexer{root: absRoot}, nil
}

// Index loads all packages under the root and rebuilds the symbol index.
// It can be called again to re-scan after source changes.
func (idx *Indexer) Index() error {
	fset := token.NewFileSet()
	cfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedSyntax |
			packages.NeedTypes |
			packages.NeedTypesInfo |
			packages.NeedDeps |
			packages.NeedImports,
		Dir:  idx.root,
		Fset: fset,
	}

	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return fmt.Errorf("loading packages: %w", err)
	}

	idx.fset = fset
	idx.PkgInfos = make(map[string]*symtab.PackageInfo, len(pkgs))
	idx.TypePkgs = make(map[string]*types.Package, len(pkgs))

	for _, pkg := range pkgs {
		if pkg.Types == nil {
			continue
		}
		// Store every loaded package for type-checking (needed for Implements checks).
		idx.TypePkgs[pkg.PkgPath] = pkg.Types

		// Only index packages whose source files live under the root directory.
		if len(pkg.GoFiles) > 0 && isUnderRoot(pkg.GoFiles[0], idx.root) {
			idx.indexPackage(pkg)
		}
	}

	return nil
}

// indexPackage processes a single package and adds it to the index.
func (idx *Indexer) indexPackage(pkg *packages.Package) {
	docs := idx.buildDocMap(pkg.Syntax)
	fieldDocs := idx.buildFieldDocMap(pkg.Syntax)

	dir := ""
	if len(pkg.GoFiles) > 0 {
		dir = filepath.Dir(pkg.GoFiles[0])
	}

	files := make([]string, len(pkg.GoFiles))
	copy(files, pkg.GoFiles)

	info := &symtab.PackageInfo{
		ImportPath: pkg.PkgPath,
		Name:       pkg.Name,
		Dir:        dir,
		Files:      files,
	}

	scope := pkg.Types.Scope()
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		switch o := obj.(type) {
		case *types.Func:
			info.Funcs = append(info.Funcs, idx.FuncInfo(o, pkg.PkgPath, docs))
		case *types.TypeName:
			info.Types = append(info.Types, idx.TypeInfo(o, pkg, docs, fieldDocs))
		case *types.Var:
			info.Vars = append(info.Vars, idx.varInfo(o, pkg.PkgPath, docs, false))
		case *types.Const:
			info.Vars = append(info.Vars, idx.varInfo(o, pkg.PkgPath, docs, true))
		}
	}

	idx.PkgInfos[pkg.PkgPath] = info
}

// FuncInfo extracts symtab.FuncInfo from a *types.Func.
func (idx *Indexer) FuncInfo(fn *types.Func, pkgPath string, docs map[token.Pos]string) symtab.FuncInfo {
	sig, ok := fn.Type().(*types.Signature)
	if !ok {
		return symtab.FuncInfo{}
	}
	pos := idx.fset.Position(fn.Pos())
	return symtab.FuncInfo{
		Name:      fn.Name(),
		Package:   pkgPath,
		Receiver:  idx.receiverString(sig),
		Signature: idx.buildSignature(fn.Name(), sig.Recv(), sig),
		Doc:       docs[fn.Pos()],
		Location:  symtab.Location{File: pos.Filename, Line: pos.Line},
	}
}

// TypeInfo extracts symtab.TypeInfo from a *types.TypeName.
func (idx *Indexer) TypeInfo(tn *types.TypeName, pkg *packages.Package, docs, fieldDocs map[token.Pos]string) symtab.TypeInfo {
	pos := idx.fset.Position(tn.Pos())
	ti := symtab.TypeInfo{
		Name:     tn.Name(),
		Package:  pkg.PkgPath,
		Doc:      docs[tn.Pos()],
		Location: symtab.Location{File: pos.Filename, Line: pos.Line},
	}

	named, ok := tn.Type().(*types.Named)
	if !ok {
		ti.Kind = symtab.TypeKindAlias
		return ti
	}

	switch u := named.Underlying().(type) {
	case *types.Struct:
		ti.Kind = symtab.TypeKindStruct
		ti.Fields, ti.Embeds = idx.structFields(u, fieldDocs)
		ti.Methods = idx.namedMethods(named, pkg.PkgPath, docs)
	case *types.Interface:
		ti.Kind = symtab.TypeKindInterface
		ti.Methods = idx.interfaceMethods(u, pkg.PkgPath, docs)
		ti.Embeds = idx.interfaceEmbeds(u)
	default:
		if tn.IsAlias() {
			ti.Kind = symtab.TypeKindAlias
		} else {
			ti.Kind = symtab.TypeKindOther
		}
		ti.Methods = idx.namedMethods(named, pkg.PkgPath, docs)
	}

	return ti
}

// varInfo extracts VarInfo from a types.Object (variable or constant).
func (idx *Indexer) varInfo(obj types.Object, pkgPath string, docs map[token.Pos]string, isConst bool) symtab.VarInfo {
	pos := idx.fset.Position(obj.Pos())
	return symtab.VarInfo{
		Name:     obj.Name(),
		Package:  pkgPath,
		Type:     types.TypeString(obj.Type(), nil),
		IsConst:  isConst,
		Doc:      docs[obj.Pos()],
		Location: symtab.Location{File: pos.Filename, Line: pos.Line},
	}
}

// structFields separates a struct's named fields from its embedded types.
func (idx *Indexer) structFields(s *types.Struct, fieldDocs map[token.Pos]string) (fields []symtab.FieldInfo, embeds []string) {
	for i := range s.NumFields() {
		f := s.Field(i)
		if f.Anonymous() {
			embeds = append(embeds, types.TypeString(f.Type(), nil))
			continue
		}
		fields = append(fields, symtab.FieldInfo{
			Name:    f.Name(),
			Type:    types.TypeString(f.Type(), nil),
			Tag:     s.Tag(i),
			Comment: fieldDocs[f.Pos()],
		})
	}
	return fields, embeds
}

// namedMethods returns all explicitly declared methods on a named type.
func (idx *Indexer) namedMethods(named *types.Named, pkgPath string, docs map[token.Pos]string) []symtab.FuncInfo {
	result := make([]symtab.FuncInfo, 0, named.NumMethods())
	for m := range named.Methods() {
		result = append(result, idx.FuncInfo(m, pkgPath, docs))
	}
	return result
}

// interfaceMethods returns the explicitly declared methods of an interface type.
func (idx *Indexer) interfaceMethods(iface *types.Interface, pkgPath string, docs map[token.Pos]string) []symtab.FuncInfo {
	result := make([]symtab.FuncInfo, 0, iface.NumExplicitMethods())
	for m := range iface.ExplicitMethods() {
		result = append(result, idx.FuncInfo(m, pkgPath, docs))
	}
	return result
}

// interfaceEmbeds returns the type strings of types embedded in an interface.
func (idx *Indexer) interfaceEmbeds(iface *types.Interface) []string {
	if iface.NumEmbeddeds() == 0 {
		return nil
	}
	result := make([]string, 0, iface.NumEmbeddeds())
	for t := range iface.EmbeddedTypes() {
		result = append(result, types.TypeString(t, nil))
	}
	return result
}

// receiverString returns the type string of a method receiver, or empty for plain functions.
func (idx *Indexer) receiverString(sig *types.Signature) string {
	recv := sig.Recv()
	if recv == nil {
		return ""
	}
	return types.TypeString(recv.Type(), nil)
}

// isUnderRoot reports whether path is within root (both should be absolute).
func isUnderRoot(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(rel, "..")
}

// buildSignature formats a function or method signature as a Go source string.
func (idx *Indexer) buildSignature(name string, recv *types.Var, sig *types.Signature) string {
	// types.TypeString gives "func(params) results" — reuse everything after "func"
	rest := types.TypeString(sig, nil)[len("func"):]

	if recv == nil {
		return "func " + name + rest
	}

	recvType := types.TypeString(recv.Type(), nil)
	if recv.Name() == "" || recv.Name() == "_" {
		return "func (" + recvType + ") " + name + rest
	}
	return "func (" + recv.Name() + " " + recvType + ") " + name + rest
}

// buildDocMap extracts doc comments for top-level declarations, keyed by the name's position.
func (idx *Indexer) buildDocMap(files []*ast.File) map[token.Pos]string {
	docs := make(map[token.Pos]string)
	for _, f := range files {
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Doc != nil {
					docs[d.Name.Pos()] = strings.TrimSpace(d.Doc.Text())
				}
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.TypeSpec:
						if doc := idx.specDoc(s.Doc, d.Doc, len(d.Specs)); doc != "" {
							docs[s.Name.Pos()] = doc
						}
					case *ast.ValueSpec:
						if doc := idx.specDoc(s.Doc, d.Doc, len(d.Specs)); doc != "" {
							for _, name := range s.Names {
								docs[name.Pos()] = doc
							}
						}
					}
				}
			}
		}
	}
	return docs
}

// buildFieldDocMap extracts comments for struct fields, keyed by field name position.
func (idx *Indexer) buildFieldDocMap(files []*ast.File) map[token.Pos]string {
	docs := make(map[token.Pos]string)
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			field, ok := n.(*ast.Field)
			if !ok {
				return true
			}
			comment := ""
			if field.Doc != nil {
				comment = strings.TrimSpace(field.Doc.Text())
			} else if field.Comment != nil {
				comment = strings.TrimSpace(field.Comment.Text())
			}
			if comment != "" {
				for _, name := range field.Names {
					docs[name.Pos()] = comment
				}
			}
			return true
		})
	}
	return docs
}

// specDoc returns the doc comment for a spec within a GenDecl.
// It prefers the spec's own doc, falling back to the group doc for single-spec decls.
func (idx *Indexer) specDoc(specDoc, groupDoc *ast.CommentGroup, specCount int) string {
	if specDoc != nil {
		return strings.TrimSpace(specDoc.Text())
	}
	if groupDoc != nil && specCount == 1 {
		return strings.TrimSpace(groupDoc.Text())
	}
	return ""
}
