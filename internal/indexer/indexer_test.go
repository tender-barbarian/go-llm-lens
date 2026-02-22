package indexer

import (
	"go/ast"
	"go/token"
	"go/types"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tender-barbarian/go-llm-lens/internal/symtab"
)

func TestIsUnderRoot(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		root     string
		expected bool
	}{
		{"direct child", "/root/foo.go", "/root", true},
		{"nested child", "/root/pkg/sub/foo.go", "/root", true},
		{"sibling dir", "/other/foo.go", "/root", false},
		{"parent dir", "/foo.go", "/root", false},
		{"root itself", "/root", "/root", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, isUnderRoot(tc.path, tc.root))
		})
	}
}

func TestSpecDoc(t *testing.T) {
	idx := &Indexer{}
	groupDoc := &ast.CommentGroup{List: []*ast.Comment{{Text: "// group doc"}}}
	specDocCG := &ast.CommentGroup{List: []*ast.Comment{{Text: "// spec doc"}}}

	tests := []struct {
		name      string
		specDoc   *ast.CommentGroup
		groupDoc  *ast.CommentGroup
		specCount int
		expected  string
	}{
		{"spec doc wins over group", specDocCG, groupDoc, 1, "spec doc"},
		{"group doc used for single spec", nil, groupDoc, 1, "group doc"},
		{"group doc ignored for multi-spec", nil, groupDoc, 3, ""},
		{"both nil", nil, nil, 1, ""},
		{"spec doc with no group", specDocCG, nil, 2, "spec doc"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, idx.specDoc(tc.specDoc, tc.groupDoc, tc.specCount))
		})
	}
}

func TestFormatSignature(t *testing.T) {
	idx := &Indexer{}

	// Named type S used as receiver – nil package so TypeString omits the path prefix.
	sObj := types.NewTypeName(token.NoPos, nil, "S", nil)
	sType := types.NewNamed(sObj, types.NewStruct(nil, nil), nil)
	ptrSType := types.NewPointer(sType)

	intParam := types.NewVar(token.NoPos, nil, "n", types.Typ[types.Int])
	boolResult := types.NewVar(token.NoPos, nil, "", types.Typ[types.Bool])

	tests := []struct {
		name     string
		fn       string
		recv     *types.Var
		sig      *types.Signature
		expected string
	}{
		{
			name:     "plain function, no params or results",
			fn:       "Foo",
			recv:     nil,
			sig:      types.NewSignatureType(nil, nil, nil, nil, nil, false),
			expected: "func Foo()",
		},
		{
			name: "plain function, with params and result",
			fn:   "Bar",
			recv: nil,
			sig: types.NewSignatureType(nil, nil, nil,
				types.NewTuple(intParam), types.NewTuple(boolResult), false),
			expected: "func Bar(n int) bool",
		},
		{
			name: "method with named receiver",
			fn:   "Method",
			recv: types.NewVar(token.NoPos, nil, "s", ptrSType),
			sig: types.NewSignatureType(nil, nil, nil,
				types.NewTuple(intParam), types.NewTuple(boolResult), false),
			expected: "func (s *S) Method(n int) bool",
		},
		{
			name: "method with blank receiver",
			fn:   "Method",
			recv: types.NewVar(token.NoPos, nil, "_", ptrSType),
			sig: types.NewSignatureType(nil, nil, nil,
				types.NewTuple(intParam), types.NewTuple(boolResult), false),
			expected: "func (*S) Method(n int) bool",
		},
		{
			name: "method with unnamed receiver",
			fn:   "Method",
			recv: types.NewVar(token.NoPos, nil, "", ptrSType),
			sig: types.NewSignatureType(nil, nil, nil,
				types.NewTuple(intParam), types.NewTuple(boolResult), false),
			expected: "func (*S) Method(n int) bool",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, idx.buildSignature(tc.fn, tc.recv, tc.sig))
		})
	}
}

func TestIndex(t *testing.T) {
	idx, err := New("../../tests/testdata")
	require.NoError(t, err)
	require.NoError(t, idx.Index())

	pkgs := idx.PkgInfos()
	require.Len(t, pkgs, 1)

	pkg := pkgs["example.com/testdata/greeter"]
	require.NotNil(t, pkg)

	assert.Equal(t, "greeter", pkg.Name)
	assert.Equal(t, "example.com/testdata/greeter", pkg.ImportPath)
	assert.Len(t, pkg.Files, 1)
	assert.Len(t, pkg.Funcs, 6)
	assert.Len(t, pkg.Types, 5)
	require.Len(t, pkg.Vars, 2)

	findFunc := func(name string) *symtab.FuncInfo {
		for i := range pkg.Funcs {
			if pkg.Funcs[i].Name == name {
				return &pkg.Funcs[i]
			}
		}
		return nil
	}
	findType := func(name string) *symtab.TypeInfo {
		for i := range pkg.Types {
			if pkg.Types[i].Name == name {
				return &pkg.Types[i]
			}
		}
		return nil
	}

	// Spot-check a function: signature and doc comment.
	newFn := findFunc("New")
	require.NotNil(t, newFn)
	assert.Contains(t, newFn.Signature, "func New(prefix string)")
	assert.Contains(t, newFn.Doc, "returns an English greeter")

	// Spot-check the Greeter interface.
	greeter := findType("Greeter")
	require.NotNil(t, greeter)
	assert.Equal(t, symtab.TypeKindInterface, greeter.Kind)
	assert.Contains(t, greeter.Doc, "interface for producing greetings")
	assert.Len(t, greeter.Methods, 1)
	assert.Equal(t, "Greet", greeter.Methods[0].Name)

	// Spot-check English: struct with one field and two methods.
	english := findType("English")
	require.NotNil(t, english)
	assert.Equal(t, symtab.TypeKindStruct, english.Kind)
	assert.Len(t, english.Fields, 1)
	assert.Equal(t, "Prefix", english.Fields[0].Name)
	assert.Len(t, english.Methods, 2)

	// Spot-check a const and a var.
	assert.True(t, pkg.Vars[0].IsConst || pkg.Vars[1].IsConst, "expected DefaultPrefix to be a const")
	assert.False(t, pkg.Vars[0].IsConst && pkg.Vars[1].IsConst, "expected MaxLength to be a var")

	// Lockable embeds sync.Mutex — its methods are promoted from a different package.
	lockable := findType("Lockable")
	require.NotNil(t, lockable)
	assert.Equal(t, symtab.TypeKindStruct, lockable.Kind)
	assert.Equal(t, []string{"sync.Mutex"}, lockable.Embeds)
	require.NotEmpty(t, lockable.Methods)
	for _, m := range lockable.Methods {
		assert.True(t, m.IsPromoted, "method %q on Lockable should be promoted", m.Name)
	}

	// FormalEnglish embeds Formal (same package) — its Greet method is still promoted.
	formalEnglish := findType("FormalEnglish")
	require.NotNil(t, formalEnglish)
	assert.Equal(t, symtab.TypeKindStruct, formalEnglish.Kind)
	require.Len(t, formalEnglish.Methods, 1)
	assert.Equal(t, "Greet", formalEnglish.Methods[0].Name)
	assert.True(t, formalEnglish.Methods[0].IsPromoted)
}
