package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/mark3labs/mcp-go/server"

	"github.com/tender-barbarian/go-llm-lens/internal/finder"
	"github.com/tender-barbarian/go-llm-lens/internal/indexer"
	"github.com/tender-barbarian/go-llm-lens/internal/tools"
)

var version = "dev"

const parentCheckInterval = 5 * time.Second

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go watchParent(ctx, cancel)

	runErr := make(chan error)
	go func() {
		runErr <- run()
	}()

	select {
	case err := <-runErr:
		if err != nil {
			log.Fatal(err)
		}
	case <-ctx.Done():
		os.Exit(0)
	}
}

func watchParent(ctx context.Context, cancel context.CancelFunc) {
	ticker := time.NewTicker(parentCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if os.Getppid() == 1 {
				cancel()
				return
			}
		}
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

	s := server.NewMCPServer("go-llm-lens", version)
	tools.Register(s, f)

	if err := server.ServeStdio(s); err != nil {
		return fmt.Errorf("serving MCP: %w", err)
	}
	return nil
}
