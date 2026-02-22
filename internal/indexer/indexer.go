package indexer

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/printer"
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
	pkgInfos map[string]*symtab.PackageInfo
	typePkgs map[string]*types.Package // all loaded packages, including deps, for Implements checks
}

// TypePkgs returns the map of all type-checked packages keyed by import path.
// It includes transitive dependencies, not just packages under the root.
func (idx *Indexer) TypePkgs() map[string]*types.Package {
	return idx.typePkgs
}

// PkgInfos returns the map of all indexed packages keyed by import path.
func (idx *Indexer) PkgInfos() map[string]*symtab.PackageInfo {
	return idx.pkgInfos
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
	idx.pkgInfos = make(map[string]*symtab.PackageInfo, len(pkgs))
	idx.typePkgs = make(map[string]*types.Package, len(pkgs))

	for _, pkg := range pkgs {
		if pkg.Types == nil {
			continue
		}
		// Store every loaded package for type-checking (needed for Implements checks).
		idx.typePkgs[pkg.PkgPath] = pkg.Types

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
	bodies := idx.buildBodyMap(pkg.Syntax)

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
			info.Funcs = append(info.Funcs, idx.funcInfo(o, pkg.PkgPath, docs, bodies))
		case *types.TypeName:
			info.Types = append(info.Types, idx.typeInfo(o, pkg, docs, fieldDocs, bodies))
		case *types.Var:
			info.Vars = append(info.Vars, idx.varInfo(o, pkg.PkgPath, docs, false))
		case *types.Const:
			info.Vars = append(info.Vars, idx.varInfo(o, pkg.PkgPath, docs, true))
		}
	}

	idx.pkgInfos[pkg.PkgPath] = info
}

// funcInfo extracts symtab.funcInfo from a *types.Func.
func (idx *Indexer) funcInfo(fn *types.Func, pkgPath string, docs, bodies map[token.Pos]string) symtab.FuncInfo {
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
		Body:      bodies[fn.Pos()],
		Location:  symtab.Location{File: pos.Filename, Line: pos.Line},
	}
}

// typeInfo extracts symtab.typeInfo from a *types.TypeName.
func (idx *Indexer) typeInfo(tn *types.TypeName, pkg *packages.Package, docs, fieldDocs, bodies map[token.Pos]string) symtab.TypeInfo {
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
		ti.Methods = idx.namedMethods(named, pkg.PkgPath, docs, bodies)
	case *types.Interface:
		ti.Kind = symtab.TypeKindInterface
		ti.Methods = idx.interfaceMethods(u, pkg.PkgPath, docs, bodies)
		ti.Embeds = idx.interfaceEmbeds(u)
	default:
		if tn.IsAlias() {
			ti.Kind = symtab.TypeKindAlias
		} else {
			ti.Kind = symtab.TypeKindOther
		}
		ti.Methods = idx.namedMethods(named, pkg.PkgPath, docs, bodies)
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

// namedMethods returns all methods on a named type, including promoted ones.
// Promoted methods (accessed through an embedded field) are marked with IsPromoted=true.
// types.MethodSet stores selections sorted by method name, so iteration order is deterministic.
func (idx *Indexer) namedMethods(named *types.Named, pkgPath string, docs, bodies map[token.Pos]string) []symtab.FuncInfo {
	mset := types.NewMethodSet(types.NewPointer(named))
	result := make([]symtab.FuncInfo, 0, mset.Len())
	for sel := range mset.Methods() {
		fn, ok := sel.Obj().(*types.Func)
		if !ok {
			continue
		}
		fi := idx.funcInfo(fn, pkgPath, docs, bodies)
		if len(sel.Index()) > 1 {
			fi.IsPromoted = true
		}
		result = append(result, fi)
	}
	return result
}

// interfaceMethods returns the explicitly declared methods of an interface type.
func (idx *Indexer) interfaceMethods(iface *types.Interface, pkgPath string, docs, bodies map[token.Pos]string) []symtab.FuncInfo {
	result := make([]symtab.FuncInfo, 0, iface.NumExplicitMethods())
	for m := range iface.ExplicitMethods() {
		result = append(result, idx.funcInfo(m, pkgPath, docs, bodies))
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

// buildBodyMap extracts the full source text of each function declaration,
// keyed by the name's position (matching types.Func.Pos()).
func (idx *Indexer) buildBodyMap(files []*ast.File) map[token.Pos]string {
	bodies := make(map[token.Pos]string)
	for _, f := range files {
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			var buf bytes.Buffer
			if err := printer.Fprint(&buf, idx.fset, fd.Body); err == nil {
				bodies[fd.Name.Pos()] = buf.String()
			}
		}
	}
	return bodies
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
