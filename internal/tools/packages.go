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

// listPackagesHandler returns a handler for the list_packages tool.
// It lists all indexed packages, optionally filtered by import-path prefix.
func listPackagesHandler(f *finder.Finder) server.ToolHandlerFunc {
	return func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filter := req.GetString("filter", "")

		type pkgSummary struct {
			ImportPath string `json:"import_path"`
			Name       string `json:"name"`
			Dir        string `json:"dir"`
			FileCount  int    `json:"file_count"`
			FuncCount  int    `json:"func_count"`
			TypeCount  int    `json:"type_count"`
		}

		pkgs := f.GetPackages()
		results := make([]pkgSummary, 0, len(pkgs))
		for _, p := range pkgs {
			if filter != "" && !strings.HasPrefix(p.ImportPath, filter) {
				continue
			}
			results = append(results, pkgSummary{
				ImportPath: p.ImportPath,
				Name:       p.Name,
				Dir:        p.Dir,
				FileCount:  len(p.Files),
				FuncCount:  len(p.Funcs),
				TypeCount:  len(p.Types),
			})
		}
		return jsonResult(results)
	}
}

// getPackageSymbolsHandler returns a handler for the get_package_symbols tool.
// It returns all functions, types, and variables/constants in the given package,
// optionally including unexported symbols.
func getPackageSymbolsHandler(f *finder.Finder) server.ToolHandlerFunc {
	return func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pkgPath, err := req.RequireString("package")
		if err != nil {
			return nil, err
		}
		includeUnexported := req.GetBool("include_unexported", false)

		pkg, ok := f.GetPackage(pkgPath)
		if !ok {
			return nil, fmt.Errorf("package %q not found", pkgPath)
		}

		type result struct {
			Funcs []symtab.FuncInfo `json:"funcs"`
			Types []symtab.TypeInfo `json:"types"`
			Vars  []symtab.VarInfo  `json:"vars"`
		}
		return jsonResult(result{
			Funcs: filterFuncs(pkg.Funcs, includeUnexported),
			Types: filterTypes(pkg.Types, includeUnexported),
			Vars:  filterVars(pkg.Vars, includeUnexported),
		})
	}
}
