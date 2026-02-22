package finder

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tender-barbarian/go-llm-lens/internal/indexer"
	"github.com/tender-barbarian/go-llm-lens/internal/symtab"
)

const fixturePkg = "example.com/testdata/greeter"

func TestFindSymbol(t *testing.T) {
	idx, err := indexer.New("../../tests/testdata")
	require.NoError(t, err)
	require.NoError(t, idx.Index())
	finder := New(idx)

	tests := []struct {
		symbol            string
		mode              MatchMode
		expectedLen       int               // expected number of results; 0 means expect empty
		expectedKind      symtab.SymbolKind // empty → expect no results
		expectedSigHas    string            // substring checked in every Signature when non-empty
		expectedReceivers []string          // set of allowed receiver values; every result's Receiver must be in this set when non-empty
	}{
		{symbol: "New", expectedLen: 1, expectedKind: symtab.SymbolKindFunc, expectedSigHas: "func New("},
		{symbol: "English", expectedLen: 1, expectedKind: symtab.SymbolKindType},
		{symbol: "DefaultPrefix", expectedLen: 1, expectedKind: symtab.SymbolKindConst},
		{symbol: "MaxLength", expectedLen: 1, expectedKind: symtab.SymbolKindVar},
		{symbol: "Greet", expectedLen: 3, expectedKind: symtab.SymbolKindMethod, expectedSigHas: "func (", expectedReceivers: []string{"*" + fixturePkg + ".English", fixturePkg + ".Formal", fixturePkg + ".Greeter"}},
		{symbol: "BlankReceiver", expectedLen: 1, expectedKind: symtab.SymbolKindMethod, expectedReceivers: []string{"*" + fixturePkg + ".English"}},
		{symbol: "ThisSymbolDefinitelyDoesNotExist"},
		{symbol: "Engl", mode: MatchPrefix, expectedLen: 1, expectedKind: symtab.SymbolKindType},
		{symbol: "Length", mode: MatchContains, expectedLen: 1, expectedKind: symtab.SymbolKindVar},
	}

	for _, tt := range tests {
		t.Run(tt.symbol, func(t *testing.T) {
			refs := finder.FindSymbol(tt.symbol, tt.mode)
			if tt.expectedKind == "" {
				assert.Empty(t, refs)
				return
			}
			require.Len(t, refs, tt.expectedLen)
			for _, r := range refs {
				assert.Equal(t, fixturePkg, r.Package)
				assert.Equal(t, tt.expectedKind, r.Kind)
				if tt.expectedSigHas != "" {
					assert.Contains(t, r.Signature, tt.expectedSigHas)
				}
				if len(tt.expectedReceivers) > 0 {
					assert.Contains(t, tt.expectedReceivers, r.Receiver)
				}
				if r.Kind != symtab.SymbolKindMethod {
					assert.Empty(t, r.Receiver)
				}
			}
		})
	}
}

func TestFindImplementations(t *testing.T) {
	idx, err := indexer.New("../../tests/testdata")
	require.NoError(t, err)
	require.NoError(t, idx.Index())
	finder := New(idx)

	tests := []struct {
		name          string
		pkgPath       string
		iface         string
		expectedErr   string
		expectedNames []string
	}{
		{
			name:          "finds concrete implementors",
			pkgPath:       fixturePkg,
			iface:         "Greeter",
			expectedNames: []string{"English", "Formal", "FormalEnglish"},
		},
		{
			name:        "package not found",
			pkgPath:     "no/such/package",
			iface:       "SomeIface",
			expectedErr: "not found in index",
		},
		{
			name:        "symbol not found",
			pkgPath:     fixturePkg,
			iface:       "NoSuchSymbol",
			expectedErr: "not found in package",
		},
		{
			name:        "symbol is a func not a type",
			pkgPath:     fixturePkg,
			iface:       "New",
			expectedErr: "is not a type",
		},
		{
			name:        "type is a struct not an interface",
			pkgPath:     fixturePkg,
			iface:       "English",
			expectedErr: "is not an interface type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			impls, err := finder.FindImplementations(tt.pkgPath, tt.iface)
			if tt.expectedErr != "" {
				assert.ErrorContains(t, err, tt.expectedErr)
				return
			}
			require.NoError(t, err)
			names := make([]string, len(impls))
			for i, ti := range impls {
				names[i] = ti.Name
			}
			assert.ElementsMatch(t, tt.expectedNames, names)
		})
	}
}
