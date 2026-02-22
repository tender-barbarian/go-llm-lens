package finder

import (
	"fmt"
	"go/types"
	"strings"

	"github.com/tender-barbarian/go-llm-lens/internal/indexer"
	"github.com/tender-barbarian/go-llm-lens/internal/symtab"
)

// MatchMode controls how symbol names are compared in FindSymbol.
type MatchMode string

const (
	MatchExact    MatchMode = "exact"
	MatchPrefix   MatchMode = "prefix"
	MatchContains MatchMode = "contains"
)

func matchesQuery(symbolName, query string, mode MatchMode) bool {
	switch mode {
	case MatchPrefix:
		return strings.HasPrefix(symbolName, query)
	case MatchContains:
		return strings.Contains(symbolName, query)
	default:
		return symbolName == query
	}
}

// Finder queries an Indexer for symbols and type relationships across indexed packages.
type Finder struct {
	idx *indexer.Indexer
}

// New creates a Finder backed by the given Indexer.
func New(idx *indexer.Indexer) *Finder {
	return &Finder{idx: idx}
}

// FindSymbol searches for symbols matching name across all indexed packages.
// It matches package-level functions, types, variables, constants, and methods.
// mode controls how name is compared: exact (default), prefix, or contains.
// All matches are returned; the caller can filter by Kind or Package.
func (f *Finder) FindSymbol(name string, mode MatchMode) []symtab.SymbolRef {
	var refs []symtab.SymbolRef
	for _, pkg := range f.idx.PkgInfos() {
		refs = append(refs, refsFromFuncs(pkg, name, mode)...)
		refs = append(refs, refsFromTypes(pkg, name, mode)...)
		refs = append(refs, refsFromVars(pkg, name, mode)...)
	}
	return refs
}

// refsFromFuncs returns symtab.SymbolRefs for package-level functions matching name.
func refsFromFuncs(pkg *symtab.PackageInfo, name string, mode MatchMode) []symtab.SymbolRef {
	var refs []symtab.SymbolRef
	for i := range pkg.Funcs {
		f := &pkg.Funcs[i]
		if !matchesQuery(f.Name, name, mode) {
			continue
		}
		refs = append(refs, symtab.SymbolRef{
			Name:      f.Name,
			Package:   pkg.ImportPath,
			Kind:      symtab.SymbolKindFunc,
			Signature: f.Signature,
			Location:  f.Location,
		})
	}
	return refs
}

// refsFromTypes returns symtab.SymbolRefs for named types and their methods matching name.
func refsFromTypes(pkg *symtab.PackageInfo, name string, mode MatchMode) []symtab.SymbolRef {
	var refs []symtab.SymbolRef
	for i := range pkg.Types {
		t := &pkg.Types[i]
		if matchesQuery(t.Name, name, mode) {
			refs = append(refs, symtab.SymbolRef{
				Name:     t.Name,
				Package:  pkg.ImportPath,
				Kind:     symtab.SymbolKindType,
				Location: t.Location,
			})
		}
		for j := range t.Methods {
			m := &t.Methods[j]
			if !matchesQuery(m.Name, name, mode) {
				continue
			}
			refs = append(refs, symtab.SymbolRef{
				Name:      m.Name,
				Package:   pkg.ImportPath,
				Kind:      symtab.SymbolKindMethod,
				Receiver:  m.Receiver,
				Signature: m.Signature,
				Location:  m.Location,
			})
		}
	}
	return refs
}

// refsFromVars returns symtab.SymbolRefs for package-level variables and constants matching name.
func refsFromVars(pkg *symtab.PackageInfo, name string, mode MatchMode) []symtab.SymbolRef {
	var refs []symtab.SymbolRef
	for i := range pkg.Vars {
		v := &pkg.Vars[i]
		if !matchesQuery(v.Name, name, mode) {
			continue
		}
		kind := symtab.SymbolKindVar
		if v.IsConst {
			kind = symtab.SymbolKindConst
		}
		refs = append(refs, symtab.SymbolRef{
			Name:     v.Name,
			Package:  pkg.ImportPath,
			Kind:     kind,
			Location: v.Location,
		})
	}
	return refs
}

// FindImplementations returns all concrete types in the indexed codebase that implement
// the named interface. It uses symtab.Implements for precise, type-system-accurate results.
func (f *Finder) FindImplementations(pkgPath, ifaceName string) ([]symtab.TypeInfo, error) {
	typePkgs := f.idx.TypePkgs()

	typPkg, ok := typePkgs[pkgPath]
	if !ok {
		return nil, fmt.Errorf("package %q not found in index", pkgPath)
	}

	obj := typPkg.Scope().Lookup(ifaceName)
	if obj == nil {
		return nil, fmt.Errorf("symbol %q not found in package %q", ifaceName, pkgPath)
	}

	tn, ok := obj.(*types.TypeName)
	if !ok {
		return nil, fmt.Errorf("%q is not a type", ifaceName)
	}

	iface, ok := tn.Type().Underlying().(*types.Interface)
	if !ok {
		return nil, fmt.Errorf("%q is not an interface type", ifaceName)
	}

	var result []symtab.TypeInfo
	for _, pkgInfo := range f.idx.PkgInfos() {
		tp, ok := typePkgs[pkgInfo.ImportPath]
		if !ok {
			continue
		}
		for _, ti := range pkgInfo.Types {
			if ti.Kind == symtab.TypeKindInterface {
				continue
			}
			obj2 := tp.Scope().Lookup(ti.Name)
			if obj2 == nil {
				continue
			}
			T := obj2.Type()
			if types.Implements(T, iface) || types.Implements(types.NewPointer(T), iface) {
				result = append(result, ti)
			}
		}
	}
	return result, nil
}

// GetPackages returns all indexed packages.
func (f *Finder) GetPackages() []*symtab.PackageInfo {
	pkgs := f.idx.PkgInfos()
	result := make([]*symtab.PackageInfo, 0, len(pkgs))
	for _, p := range pkgs {
		result = append(result, p)
	}
	return result
}

// GetPackage returns a package by import path.
func (f *Finder) GetPackage(importPath string) (*symtab.PackageInfo, bool) {
	p, ok := f.idx.PkgInfos()[importPath]
	return p, ok
}
