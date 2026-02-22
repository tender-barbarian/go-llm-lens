package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/tender-barbarian/go-llm-lens/internal/finder"
	"github.com/tender-barbarian/go-llm-lens/internal/symtab"
)

// findSymbolHandler returns a handler for the find_symbol tool.
// It searches for an exact symbol name across all indexed packages,
// with an optional kind filter (func, method, type, var, const).
func findSymbolHandler(f *finder.Finder) server.ToolHandlerFunc {
	return func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := req.RequireString("name")
		if err != nil {
			return nil, err
		}
		kind := req.GetString("kind", "")
		match := finder.MatchMode(req.GetString("match", string(finder.MatchExact)))

		refs := f.FindSymbol(name, match)
		if kind != "" {
			filtered := make([]symtab.SymbolRef, 0, len(refs))
			for _, r := range refs {
				if string(r.Kind) == kind {
					filtered = append(filtered, r)
				}
			}
			refs = filtered
		}
		return jsonResult(refs)
	}
}

// getFunctionHandler returns a handler for the get_function tool.
// It looks up a package-level function or, when name is "TypeName.MethodName",
// a method on a named type.
func getFunctionHandler(f *finder.Finder) server.ToolHandlerFunc {
	return func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pkgPath, err := req.RequireString("package")
		if err != nil {
			return nil, err
		}
		name, err := req.RequireString("name")
		if err != nil {
			return nil, err
		}

		pkg, ok := f.GetPackage(pkgPath)
		if !ok {
			return nil, fmt.Errorf("package %q not found", pkgPath)
		}

		// Method: TypeName.MethodName
		if typeName, methodName, ok := strings.Cut(name, "."); ok {
			for i := range pkg.Types {
				t := &pkg.Types[i]
				if t.Name != typeName {
					continue
				}
				for _, m := range t.Methods {
					if m.Name == methodName {
						return jsonResult(m)
					}
				}
				return nil, fmt.Errorf("method %q not found on type %q in package %q", methodName, typeName, pkgPath)
			}
			return nil, fmt.Errorf("type %q not found in package %q", typeName, pkgPath)
		}

		// Package-level function
		for _, fn := range pkg.Funcs {
			if fn.Name == name {
				return jsonResult(fn)
			}
		}
		return nil, fmt.Errorf("function %q not found in package %q", name, pkgPath)
	}
}

// getTypeHandler returns a handler for the get_type tool.
// It looks up a named type (struct, interface, or other) in the given package
// and returns its full definition including fields, methods, and doc comment.
func getTypeHandler(f *finder.Finder) server.ToolHandlerFunc {
	return func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pkgPath, err := req.RequireString("package")
		if err != nil {
			return nil, err
		}
		name, err := req.RequireString("name")
		if err != nil {
			return nil, err
		}

		pkg, ok := f.GetPackage(pkgPath)
		if !ok {
			return nil, fmt.Errorf("package %q not found", pkgPath)
		}

		for i := range pkg.Types {
			t := &pkg.Types[i]
			if t.Name == name {
				return jsonResult(t)
			}
		}
		return nil, fmt.Errorf("type %q not found in package %q", name, pkgPath)
	}
}
