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
		symbol     string
		wantLen    int               // expected number of results; 0 means expect empty
		wantKind   symtab.SymbolKind // empty → expect no results
		wantSigHas string            // substring checked in every Signature when non-empty
	}{
		{symbol: "New", wantLen: 1, wantKind: symtab.SymbolKindFunc, wantSigHas: "func New("},
		{symbol: "English", wantLen: 1, wantKind: symtab.SymbolKindType},
		{symbol: "DefaultPrefix", wantLen: 1, wantKind: symtab.SymbolKindConst},
		{symbol: "MaxLength", wantLen: 1, wantKind: symtab.SymbolKindVar},
		{symbol: "Greet", wantLen: 3, wantKind: symtab.SymbolKindMethod, wantSigHas: "func ("},
		{symbol: "ThisSymbolDefinitelyDoesNotExist"},
	}

	for _, tt := range tests {
		t.Run(tt.symbol, func(t *testing.T) {
			refs := finder.FindSymbol(tt.symbol)
			if tt.wantKind == "" {
				assert.Empty(t, refs)
				return
			}
			require.Len(t, refs, tt.wantLen)
			for _, r := range refs {
				assert.Equal(t, fixturePkg, r.Package)
				assert.Equal(t, tt.wantKind, r.Kind)
				if tt.wantSigHas != "" {
					assert.Contains(t, r.Signature, tt.wantSigHas)
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
		name      string
		pkgPath   string
		iface     string
		wantErr   string
		wantNames []string
	}{
		{
			name:      "finds concrete implementors",
			pkgPath:   fixturePkg,
			iface:     "Greeter",
			wantNames: []string{"English", "Formal"},
		},
		{
			name:    "package not found",
			pkgPath: "no/such/package",
			iface:   "SomeIface",
			wantErr: "not found in index",
		},
		{
			name:    "symbol not found",
			pkgPath: fixturePkg,
			iface:   "NoSuchSymbol",
			wantErr: "not found in package",
		},
		{
			name:    "symbol is a func not a type",
			pkgPath: fixturePkg,
			iface:   "New",
			wantErr: "is not a type",
		},
		{
			name:    "type is a struct not an interface",
			pkgPath: fixturePkg,
			iface:   "English",
			wantErr: "is not an interface type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			impls, err := finder.FindImplementations(tt.pkgPath, tt.iface)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			names := make([]string, len(impls))
			for i, ti := range impls {
				names[i] = ti.Name
			}
			assert.ElementsMatch(t, tt.wantNames, names)
		})
	}
}
