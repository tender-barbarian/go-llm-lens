package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tender-barbarian/go-llm-lens/internal/finder"
	"github.com/tender-barbarian/go-llm-lens/internal/indexer"
	"github.com/tender-barbarian/go-llm-lens/internal/symtab"
)

func TestFindSymbolHandler(t *testing.T) {
	idx, err := indexer.New(fixturePkgPath)
	require.NoError(t, err)
	require.NoError(t, idx.Index())

	handler := findSymbolHandler(finder.New(idx))

	tests := []struct {
		name     string
		symbol   string
		kind     string
		expected []symtab.SymbolRef
	}{
		{name: "package-level function", symbol: "New", expected: []symtab.SymbolRef{{Kind: symtab.SymbolKindFunc}}},
		{name: "type", symbol: "English", expected: []symtab.SymbolRef{{Kind: symtab.SymbolKindType}}},
		{name: "const", symbol: "DefaultPrefix", expected: []symtab.SymbolRef{{Kind: symtab.SymbolKindConst}}},
		{name: "var", symbol: "MaxLength", expected: []symtab.SymbolRef{{Kind: symtab.SymbolKindVar}}},
		{name: "method across types", symbol: "Greet", expected: []symtab.SymbolRef{{Kind: symtab.SymbolKindMethod}, {Kind: symtab.SymbolKindMethod}, {Kind: symtab.SymbolKindMethod}}},
		{name: "kind filter includes", symbol: "New", kind: "func", expected: []symtab.SymbolRef{{Kind: symtab.SymbolKindFunc}}},
		{name: "kind filter excludes", symbol: "New", kind: "method"},
		{name: "nonexistent symbol", symbol: "NoSuchSymbol"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := map[string]any{"name": tt.symbol}
			if tt.kind != "" {
				args["kind"] = tt.kind
			}
			req := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: args}}
			resp, err := handler(context.Background(), req)
			require.NoError(t, err)

			content, ok := resp.Content[0].(mcp.TextContent)
			require.True(t, ok)

			var actuals []symtab.SymbolRef
			err = json.Unmarshal([]byte(content.Text), &actuals)
			require.NoError(t, err)

			assert.Len(t, actuals, len(tt.expected))
			for i, actual := range actuals {
				assert.Equal(t, tt.symbol, actual.Name)
				assert.Equal(t, tt.expected[i].Kind, actual.Kind)
				assert.Equal(t, fixturePkg, actual.Package)
			}
		})
	}
}

func TestGetFunctionHandler(t *testing.T) {
	idx, err := indexer.New(fixturePkgPath)
	require.NoError(t, err)
	require.NoError(t, idx.Index())

	handler := getFunctionHandler(finder.New(idx))

	tests := []struct {
		name         string
		pkg          string
		functionName string
		expectedErr  string
		expected     *symtab.FuncInfo
	}{
		{
			name:         "package-level function",
			pkg:          fixturePkg,
			functionName: "New",
			expected:     &symtab.FuncInfo{Name: "New", Signature: "func New(prefix string)", Doc: "returns an English greeter", Body: "{\n\treturn &English{Prefix: prefix}\n}"},
		},
		{
			name:         "method lookup",
			pkg:          fixturePkg,
			functionName: "English.Greet",
			expected:     &symtab.FuncInfo{Name: "Greet", Signature: "Greet(name string) string", Body: "{\n\treturn e.Prefix + name\n}"},
		},
		{
			name:         "package not found",
			pkg:          "no/such/pkg",
			functionName: "New",
			expectedErr:  "not found",
		},
		{
			name:         "function not found",
			pkg:          fixturePkg,
			functionName: "NoSuchFunc",
			expectedErr:  `"NoSuchFunc" not found`,
		},
		{
			name:         "type not found for method",
			pkg:          fixturePkg,
			functionName: "NoSuchType.Method",
			expectedErr:  `"NoSuchType" not found`,
		},
		{
			name:         "method not found on type",
			pkg:          fixturePkg,
			functionName: "English.NoSuchMethod",
			expectedErr:  `"NoSuchMethod" not found`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{
				"package": tt.pkg,
				"name":    tt.functionName,
			}}}
			res, err := handler(context.Background(), req)
			if tt.expectedErr != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tt.expectedErr)
				return
			}
			require.NoError(t, err)

			content, ok := res.Content[0].(mcp.TextContent)
			require.True(t, ok)

			var actual symtab.FuncInfo
			err = json.Unmarshal([]byte(content.Text), &actual)
			require.NoError(t, err)
			assert.Equal(t, tt.expected.Name, actual.Name)
			if tt.expected.Signature != "" {
				assert.Contains(t, actual.Signature, tt.expected.Signature)
			}
			if tt.expected.Doc != "" {
				assert.Contains(t, actual.Doc, tt.expected.Doc)
			}
			if tt.expected.Body != "" {
				assert.Equal(t, tt.expected.Body, actual.Body)
			}
		})
	}
}

func TestGetTypeHandler(t *testing.T) {
	idx, err := indexer.New(fixturePkgPath)
	require.NoError(t, err)
	require.NoError(t, idx.Index())

	handler := getTypeHandler(finder.New(idx))

	tests := []struct {
		name        string
		pkg         string
		typeName    string
		expectedErr string
		expected    *symtab.TypeInfo
	}{
		{
			name:     "interface type",
			pkg:      fixturePkg,
			typeName: "Greeter",
			expected: &symtab.TypeInfo{Kind: symtab.TypeKindInterface, Doc: "interface for producing greetings", Methods: make([]symtab.FuncInfo, 1)},
		},
		{
			name:     "struct type with methods",
			pkg:      fixturePkg,
			typeName: "English",
			expected: &symtab.TypeInfo{Kind: symtab.TypeKindStruct, Methods: make([]symtab.FuncInfo, 2), Fields: make([]symtab.FieldInfo, 1)},
		},
		{
			name:        "package not found",
			pkg:         "no/such/pkg",
			typeName:    "Greeter",
			expectedErr: "not found",
		},
		{
			name:        "type not found",
			pkg:         fixturePkg,
			typeName:    "NoSuchType",
			expectedErr: `"NoSuchType" not found`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{
				"package": tt.pkg,
				"name":    tt.typeName,
			}}}
			res, err := handler(context.Background(), req)
			if tt.expectedErr != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tt.expectedErr)
				return
			}
			require.NoError(t, err)

			content, ok := res.Content[0].(mcp.TextContent)
			require.True(t, ok)

			var actual symtab.TypeInfo
			err = json.Unmarshal([]byte(content.Text), &actual)
			require.NoError(t, err)

			assert.Equal(t, tt.typeName, actual.Name)
			assert.Equal(t, tt.expected.Kind, actual.Kind)
			if tt.expected.Doc != "" {
				assert.Contains(t, actual.Doc, tt.expected.Doc)
			}
			assert.Len(t, actual.Methods, len(tt.expected.Methods))
			assert.Len(t, actual.Fields, len(tt.expected.Fields))
		})
	}
}
