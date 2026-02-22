package tools

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/tender-barbarian/go-llm-lens/internal/finder"
)

// Register wires all codebase-scanner MCP tools to s.
// Each tool delegates to f for querying the indexed codebase.
func Register(s *server.MCPServer, f *finder.Finder) {
	s.AddTool(mcp.NewTool("list_packages",
		mcp.WithDescription("Lists all indexed packages with summary statistics."),
		mcp.WithString("filter", mcp.Description("Optional prefix filter on import path")),
	), withLengthCheck(listPackagesHandler(f)))

	s.AddTool(mcp.NewTool("get_package_symbols",
		mcp.WithDescription("Returns all symbols in a package: functions, types, variables, and constants."),
		mcp.WithString("package", mcp.Required(), mcp.Description("Package import path")),
		mcp.WithBoolean("include_unexported", mcp.Description("Include unexported symbols (default: false)")),
	), withLengthCheck(getPackageSymbolsHandler(f)))

	s.AddTool(mcp.NewTool("find_symbol",
		mcp.WithDescription("Searches for a symbol by name across the entire indexed codebase."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Symbol name (exact match)")),
		mcp.WithString("kind", mcp.Description("Filter by kind: func, method, type, var, const (empty = all)")),
		mcp.WithString("match", mcp.Description(`Match mode: "exact" (default), "prefix", or "contains"`)),
	), withLengthCheck(findSymbolHandler(f)))

	s.AddTool(mcp.NewTool("get_function",
		mcp.WithDescription("Returns full details for a specific function or method."),
		mcp.WithString("package", mcp.Required(), mcp.Description("Package import path")),
		mcp.WithString("name", mcp.Required(), mcp.Description("Function name, or TypeName.MethodName for methods")),
	), withLengthCheck(getFunctionHandler(f)))

	s.AddTool(mcp.NewTool("get_type",
		mcp.WithDescription("Returns full definition of a type (struct or interface)."),
		mcp.WithString("package", mcp.Required(), mcp.Description("Package import path")),
		mcp.WithString("name", mcp.Required(), mcp.Description("Type name")),
	), withLengthCheck(getTypeHandler(f)))

	s.AddTool(mcp.NewTool("find_implementations",
		mcp.WithDescription("Finds all concrete types in the indexed codebase that implement a given interface."),
		mcp.WithString("package", mcp.Required(), mcp.Description("Package import path of the interface")),
		mcp.WithString("interface", mcp.Required(), mcp.Description("Interface type name")),
	), withLengthCheck(findImplementationsHandler(f)))
}
