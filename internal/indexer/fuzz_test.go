package indexer

import "testing"

func FuzzIsUnderRoot(f *testing.F) {
	f.Add("/root/foo.go", "/root")
	f.Add("/root/pkg/sub/foo.go", "/root")
	f.Add("/other/foo.go", "/root")
	f.Add("", "")
	f.Add("..", "/root")
	f.Fuzz(func(t *testing.T, path, root string) {
		isUnderRoot(path, root) // must not panic
	})
}
