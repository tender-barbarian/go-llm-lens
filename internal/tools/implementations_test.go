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

func TestFindImplementationsHandler(t *testing.T) {
	idx, err := indexer.New(fixturePkgPath)
	require.NoError(t, err)
	require.NoError(t, idx.Index())

	handler := findImplementationsHandler(finder.New(idx))

	tests := []struct {
		name        string
		pkg         string
		iface       string
		expectedErr string
		expected    []symtab.TypeInfo
	}{
		{
			name:  "finds concrete implementors",
			pkg:   fixturePkg,
			iface: "Greeter",
			expected: []symtab.TypeInfo{
				{
					Name:    "English",
					Package: fixturePkg,
					Kind:    symtab.TypeKindStruct,
					Doc:     "English greets in English using a configurable prefix.",
					Fields: []symtab.FieldInfo{
						{Name: "Prefix", Type: "string", Comment: "Prefix is prepended to the name."},
					},
					Methods: []symtab.FuncInfo{
						{
							Name:      "Greet",
							Package:   fixturePkg,
							Receiver:  "*example.com/testdata/greeter.English",
							Signature: "func (e *example.com/testdata/greeter.English) Greet(name string) string",
							Doc:       "Greet returns a greeting.",
						},
						{
							Name:      "BlankReceiver",
							Package:   fixturePkg,
							Receiver:  "*example.com/testdata/greeter.English",
							Signature: "func (*example.com/testdata/greeter.English) BlankReceiver()",
							Doc:       "BlankReceiver exercises blank-receiver signature formatting.",
						},
					},
				},
				{
					Name:    "Formal",
					Package: fixturePkg,
					Kind:    symtab.TypeKindStruct,
					Doc:     "Formal greets with a formal salutation.",
					Methods: []symtab.FuncInfo{
						{
							Name:      "Greet",
							Package:   fixturePkg,
							Receiver:  "example.com/testdata/greeter.Formal",
							Signature: "func (f example.com/testdata/greeter.Formal) Greet(name string) string",
							Doc:       "Greet returns a formal greeting.",
						},
					},
				},
			},
		},
		{
			name:        "package not found",
			pkg:         "no/such/pkg",
			iface:       "Greeter",
			expectedErr: "finding implementations",
		},
		{
			name:        "interface not found",
			pkg:         fixturePkg,
			iface:       "NoSuchInterface",
			expectedErr: "finding implementations",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{
				"package":   tt.pkg,
				"interface": tt.iface,
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

			var actual []symtab.TypeInfo
			err = json.Unmarshal([]byte(content.Text), &actual)
			require.NoError(t, err)

			// Zero out Location fields — they contain absolute paths that vary by machine.
			for i := range actual {
				actual[i].Location = symtab.Location{}
				for j := range actual[i].Methods {
					actual[i].Methods[j].Location = symtab.Location{}
				}
			}

			assert.Equal(t, tt.expected, actual)
		})
	}
}
