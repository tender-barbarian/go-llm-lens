package tools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/tender-barbarian/go-llm-lens/internal/finder"
)

// findImplementationsHandler returns a handler for the find_implementations tool.
// It uses go/types.Implements to find all concrete types in the indexed codebase
// that satisfy the named interface.
func findImplementationsHandler(f *finder.Finder) server.ToolHandlerFunc {
	return func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pkgPath, err := req.RequireString("package")
		if err != nil {
			return nil, err
		}
		ifaceName, err := req.RequireString("interface")
		if err != nil {
			return nil, err
		}

		impls, err := f.FindImplementations(pkgPath, ifaceName)
		if err != nil {
			return nil, fmt.Errorf("finding implementations of %q: %w", ifaceName, err)
		}
		return jsonResult(impls)
	}
}
