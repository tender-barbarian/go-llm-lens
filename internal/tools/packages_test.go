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

func TestListPackagesHandler(t *testing.T) {
	idx, err := indexer.New(fixturePkgPath)
	require.NoError(t, err)
	require.NoError(t, idx.Index())

	handler := listPackagesHandler(finder.New(idx))

	type pkgSummary struct {
		ImportPath string `json:"import_path"`
		Name       string `json:"name"`
		FileCount  int    `json:"file_count"`
		FuncCount  int    `json:"func_count"`
		TypeCount  int    `json:"type_count"`
	}

	tests := []struct {
		name     string
		filter   string
		expected *pkgSummary
	}{
		{name: "no filter returns all packages", expected: &pkgSummary{ImportPath: fixturePkg, Name: "greeter", FileCount: 1, FuncCount: 6, TypeCount: 3}},
		{name: "matching prefix returns package", filter: "example.com", expected: &pkgSummary{ImportPath: fixturePkg, Name: "greeter", FileCount: 1, FuncCount: 6, TypeCount: 3}},
		{name: "non-matching prefix returns empty", filter: "no/match"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := map[string]any{}
			if tt.filter != "" {
				args["filter"] = tt.filter
			}
			req := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: args}}
			res, err := handler(context.Background(), req)
			require.NoError(t, err)

			content, ok := res.Content[0].(mcp.TextContent)
			require.True(t, ok)

			var actual []pkgSummary
			err = json.Unmarshal([]byte(content.Text), &actual)
			require.NoError(t, err)

			if tt.expected == nil {
				assert.Empty(t, actual)
				return
			}
			require.NotEmpty(t, actual)
			assert.Equal(t, *tt.expected, actual[0])
		})
	}
}

func TestGetPackageSymbolsHandler(t *testing.T) {
	idx, err := indexer.New(fixturePkgPath)
	require.NoError(t, err)
	require.NoError(t, idx.Index())

	handler := getPackageSymbolsHandler(finder.New(idx))

	type symbolSet struct {
		Funcs []symtab.FuncInfo `json:"funcs"`
		Types []symtab.TypeInfo `json:"types"`
		Vars  []symtab.VarInfo  `json:"vars"`
	}

	tests := []struct {
		name              string
		pkg               string
		includeUnexported bool
		expectedErr       string
		expectedFuncs     int
		expectedTypes     int
		expectedVars      int
	}{
		{
			name:        "package not found returns error",
			pkg:         "no/such/pkg",
			expectedErr: "not found",
		},
		{
			name:          "exported symbols only",
			pkg:           fixturePkg,
			expectedFuncs: 6,
			expectedTypes: 3,
			expectedVars:  2,
		},
		{
			name:              "include_unexported=true same counts when all exported",
			pkg:               fixturePkg,
			includeUnexported: true,
			expectedFuncs:     6,
			expectedTypes:     3,
			expectedVars:      2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := map[string]any{"package": tt.pkg}
			if tt.includeUnexported {
				args["include_unexported"] = true
			}
			req := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: args}}
			res, err := handler(context.Background(), req)
			if tt.expectedErr != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tt.expectedErr)
				return
			}
			require.NoError(t, err)

			content, ok := res.Content[0].(mcp.TextContent)
			require.True(t, ok)

			var actual symbolSet
			err = json.Unmarshal([]byte(content.Text), &actual)
			require.NoError(t, err)
			assert.Len(t, actual.Funcs, tt.expectedFuncs)
			assert.Len(t, actual.Types, tt.expectedTypes)
			assert.Len(t, actual.Vars, tt.expectedVars)
		})
	}
}
