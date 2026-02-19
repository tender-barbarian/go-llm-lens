package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"go/token"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/tender-barbarian/go-llm-lens/internal/symtab"
)

const maxInputLen = 2048

// withLengthCheck wraps a handler and rejects any request that contains a
// string argument longer than maxInputLen bytes.
func withLengthCheck(next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		for field, val := range req.GetArguments() {
			s, ok := val.(string)
			if !ok {
				continue
			}
			if len(s) > maxInputLen {
				return nil, fmt.Errorf("field %q exceeds maximum length of %d bytes", field, maxInputLen)
			}
		}
		return next(ctx, req)
	}
}

// jsonResult serialises v to JSON and wraps it in a text tool result.
func jsonResult(v any) (*mcp.CallToolResult, error) {
	out, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("encoding response: %w", err)
	}
	return mcp.NewToolResultText(string(out)), nil
}

// filterFuncs returns funcs, optionally dropping unexported ones.
func filterFuncs(funcs []symtab.FuncInfo, includeUnexported bool) []symtab.FuncInfo {
	if includeUnexported {
		return funcs
	}
	result := make([]symtab.FuncInfo, 0, len(funcs))
	for _, f := range funcs {
		if token.IsExported(f.Name) {
			result = append(result, f)
		}
	}
	return result
}

// filterTypes returns types, optionally dropping unexported ones.
func filterTypes(typs []symtab.TypeInfo, includeUnexported bool) []symtab.TypeInfo {
	if includeUnexported {
		return typs
	}
	result := make([]symtab.TypeInfo, 0, len(typs))
	for _, t := range typs {
		if token.IsExported(t.Name) {
			result = append(result, t)
		}
	}
	return result
}

// filterVars returns vars, optionally dropping unexported ones.
func filterVars(vars []symtab.VarInfo, includeUnexported bool) []symtab.VarInfo {
	if includeUnexported {
		return vars
	}
	result := make([]symtab.VarInfo, 0, len(vars))
	for _, v := range vars {
		if token.IsExported(v.Name) {
			result = append(result, v)
		}
	}
	return result
}
