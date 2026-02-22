// Package greeter is a test fixture for the indexer.
package greeter

import "sync"

// Greeter is the interface for producing greetings.
type Greeter interface {
	// Greet returns a greeting string for the given name.
	Greet(name string) string
}

// English greets in English using a configurable prefix.
type English struct {
	// Prefix is prepended to the name.
	Prefix string
}

// Greet returns a greeting.
func (e *English) Greet(name string) string {
	return e.Prefix + name
}

// Formal greets with a formal salutation.
type Formal struct{}

// Greet returns a formal greeting.
func (f Formal) Greet(name string) string {
	return "Dear " + name
}

// DefaultPrefix is the default greeting prefix.
const DefaultPrefix = "Hello, "

// MaxLength is the maximum allowed greeting length.
var MaxLength = 100

// New returns an English greeter with the given prefix.
func New(prefix string) *English {
	return &English{Prefix: prefix}
}

// NoReturn does something with no return value.
func NoReturn(s string) {}

// SingleNamed returns a named result.
func SingleNamed(s string) (result string) { return s }

// MultiUnnamed returns multiple unnamed results.
func MultiUnnamed(s string) (string, error) { return s, nil }

// MultiNamed returns multiple named results.
func MultiNamed(s string) (out string, err error) { return s, nil }

// Variadic joins strings with a separator.
func Variadic(sep string, parts ...string) string { return "" }

// BlankReceiver exercises blank-receiver signature formatting.
func (_ *English) BlankReceiver() {}

// Lockable intentionally uses a public struct with embedded sync.Mutex
// to exercise cross-package promoted method detection in the indexer.
type Lockable struct {
	sync.Mutex
}

// FormalEnglish intentionally uses a public struct with embedded Formal
// to exercise same-package promoted method detection in the indexer.
type FormalEnglish struct {
	Formal
}
