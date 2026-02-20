package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tender-barbarian/go-llm-lens/internal/symtab"
)

func TestWithLengthCheck(t *testing.T) {
	handler := withLengthCheck(func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("ok"), nil
	})

	tests := []struct {
		name        string
		args        map[string]any
		expectedErr bool
	}{
		{
			name:        "short string passes through",
			args:        map[string]any{"q": "hello"},
			expectedErr: false,
		},
		{
			name:        "string at exact limit passes through",
			args:        map[string]any{"q": strings.Repeat("x", maxInputLen)},
			expectedErr: false,
		},
		{
			name:        "string one byte over limit is rejected",
			args:        map[string]any{"q": strings.Repeat("x", maxInputLen+1)},
			expectedErr: true,
		},
		{
			name:        "non-string argument is allowed",
			args:        map[string]any{"n": 42},
			expectedErr: false,
		},
		{
			name:        "nil arguments passes through",
			args:        nil,
			expectedErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: tt.args}}
			_, err := handler(context.Background(), req)
			if tt.expectedErr {
				require.Error(t, err)
				assert.ErrorContains(t, err, "exceeds maximum length")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestFilterFuncs(t *testing.T) {
	funcs := []symtab.FuncInfo{
		{Name: "Exported"},
		{Name: "unexported"},
	}

	tests := []struct {
		name              string
		includeUnexported bool
		expected          []symtab.FuncInfo
	}{
		{"exported only", false, []symtab.FuncInfo{{Name: "Exported"}}},
		{"include unexported", true, []symtab.FuncInfo{{Name: "Exported"}, {Name: "unexported"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := filterFuncs(funcs, tt.includeUnexported)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestFilterTypes(t *testing.T) {
	types := []symtab.TypeInfo{
		{Name: "MyType"},
		{Name: "myType"},
	}

	tests := []struct {
		name              string
		includeUnexported bool
		expected          []symtab.TypeInfo
	}{
		{"exported only", false, []symtab.TypeInfo{{Name: "MyType"}}},
		{"include unexported", true, []symtab.TypeInfo{{Name: "MyType"}, {Name: "myType"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := filterTypes(types, tt.includeUnexported)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestFilterVars(t *testing.T) {
	vars := []symtab.VarInfo{
		{Name: "Exported"},
		{Name: "unexported"},
	}

	tests := []struct {
		name              string
		includeUnexported bool
		expected          []symtab.VarInfo
	}{
		{"exported only", false, []symtab.VarInfo{{Name: "Exported"}}},
		{"include unexported", true, []symtab.VarInfo{{Name: "Exported"}, {Name: "unexported"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := filterVars(vars, tt.includeUnexported)
			assert.Equal(t, tt.expected, actual)
		})
	}
}
