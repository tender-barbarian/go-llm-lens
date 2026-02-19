package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/mark3labs/mcp-go/server"

	"github.com/tender-barbarian/go-llm-lens/internal/finder"
	"github.com/tender-barbarian/go-llm-lens/internal/indexer"
	"github.com/tender-barbarian/go-llm-lens/internal/tools"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	root := flag.String("root", ".", "Root directory of the Go codebase to index")
	flag.Parse()

	info, err := os.Stat(*root)
	if err != nil {
		return fmt.Errorf("invalid --root: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("--root %q is not a directory", *root)
	}

	idx, err := indexer.New(*root)
	if err != nil {
		return fmt.Errorf("creating indexer: %w", err)
	}

	fmt.Fprintln(os.Stderr, "Indexing codebase...")
	if err := idx.Index(); err != nil {
		return fmt.Errorf("indexing codebase: %w", err)
	}
	fmt.Fprintln(os.Stderr, "Index ready.")

	f := finder.New(idx)

	s := server.NewMCPServer("go-llm-lens", "0.1.0")
	tools.Register(s, f)

	if err := server.ServeStdio(s); err != nil {
		return fmt.Errorf("serving MCP: %w", err)
	}
	return nil
}
