package indexer

import (
	"go/ast"
	"go/token"
	"go/types"
	"testing"

	"github.com/stretchr/testify/assert"
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
		want      string
	}{
		{"spec doc wins over group", specDocCG, groupDoc, 1, "spec doc"},
		{"group doc used for single spec", nil, groupDoc, 1, "group doc"},
		{"group doc ignored for multi-spec", nil, groupDoc, 3, ""},
		{"both nil", nil, nil, 1, ""},
		{"spec doc with no group", specDocCG, nil, 2, "spec doc"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, idx.specDoc(tc.specDoc, tc.groupDoc, tc.specCount))
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
		name string
		fn   string
		recv *types.Var
		sig  *types.Signature
		want string
	}{
		{
			name: "plain function, no params or results",
			fn:   "Foo",
			recv: nil,
			sig:  types.NewSignatureType(nil, nil, nil, nil, nil, false),
			want: "func Foo()",
		},
		{
			name: "plain function, with params and result",
			fn:   "Bar",
			recv: nil,
			sig: types.NewSignatureType(nil, nil, nil,
				types.NewTuple(intParam), types.NewTuple(boolResult), false),
			want: "func Bar(n int) bool",
		},
		{
			name: "method with named receiver",
			fn:   "Method",
			recv: types.NewVar(token.NoPos, nil, "s", ptrSType),
			sig: types.NewSignatureType(nil, nil, nil,
				types.NewTuple(intParam), types.NewTuple(boolResult), false),
			want: "func (s *S) Method(n int) bool",
		},
		{
			name: "method with blank receiver",
			fn:   "Method",
			recv: types.NewVar(token.NoPos, nil, "_", ptrSType),
			sig: types.NewSignatureType(nil, nil, nil,
				types.NewTuple(intParam), types.NewTuple(boolResult), false),
			want: "func (*S) Method(n int) bool",
		},
		{
			name: "method with unnamed receiver",
			fn:   "Method",
			recv: types.NewVar(token.NoPos, nil, "", ptrSType),
			sig: types.NewSignatureType(nil, nil, nil,
				types.NewTuple(intParam), types.NewTuple(boolResult), false),
			want: "func (*S) Method(n int) bool",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, idx.buildSignature(tc.fn, tc.recv, tc.sig))
		})
	}
}
