package finder

import (
	"fmt"
	"go/types"

	"github.com/tender-barbarian/go-llm-lens/internal/indexer"
	"github.com/tender-barbarian/go-llm-lens/internal/symtab"
)

// Finder queries an Indexer for symbols and type relationships across indexed packages.
type Finder struct {
	idx *indexer.Indexer
}

// New creates a Finder backed by the given Indexer.
func New(idx *indexer.Indexer) *Finder {
	return &Finder{idx: idx}
}

// FindSymbol searches for a symbol by exact name across all indexed packages.
// It matches package-level functions, types, variables, constants, and methods.
// All matches are returned; the caller can filter by Kind or Package.
func (f *Finder) FindSymbol(name string) []symtab.SymbolRef {
	var refs []symtab.SymbolRef
	for _, pkg := range f.idx.PkgInfos {
		refs = append(refs, refsFromFuncs(pkg, name)...)
		refs = append(refs, refsFromTypes(pkg, name)...)
		refs = append(refs, refsFromVars(pkg, name)...)
	}
	return refs
}

// refsFromFuncs returns symtab.SymbolRefs for package-level functions matching name.
func refsFromFuncs(pkg *symtab.PackageInfo, name string) []symtab.SymbolRef {
	var refs []symtab.SymbolRef
	for i := range pkg.Funcs {
		f := &pkg.Funcs[i]
		if f.Name != name {
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
func refsFromTypes(pkg *symtab.PackageInfo, name string) []symtab.SymbolRef {
	var refs []symtab.SymbolRef
	for i := range pkg.Types {
		t := &pkg.Types[i]
		if t.Name == name {
			refs = append(refs, symtab.SymbolRef{
				Name:     t.Name,
				Package:  pkg.ImportPath,
				Kind:     symtab.SymbolKindType,
				Location: t.Location,
			})
		}
		for j := range t.Methods {
			m := &t.Methods[j]
			if m.Name != name {
				continue
			}
			refs = append(refs, symtab.SymbolRef{
				Name:      m.Name,
				Package:   pkg.ImportPath,
				Kind:      symtab.SymbolKindMethod,
				Signature: m.Signature,
				Location:  m.Location,
			})
		}
	}
	return refs
}

// refsFromVars returns symtab.SymbolRefs for package-level variables and constants matching name.
func refsFromVars(pkg *symtab.PackageInfo, name string) []symtab.SymbolRef {
	var refs []symtab.SymbolRef
	for i := range pkg.Vars {
		v := &pkg.Vars[i]
		if v.Name != name {
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
	typPkg, ok := f.idx.TypePkgs[pkgPath]
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
	for _, pkgInfo := range f.idx.PkgInfos {
		tp, ok := f.idx.TypePkgs[pkgInfo.ImportPath]
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
	result := make([]*symtab.PackageInfo, 0, len(f.idx.PkgInfos))
	for _, p := range f.idx.PkgInfos {
		result = append(result, p)
	}
	return result
}

// GetPackage returns a package by import path.
func (f *Finder) GetPackage(importPath string) (*symtab.PackageInfo, bool) {
	p, ok := f.idx.PkgInfos[importPath]
	return p, ok
}
