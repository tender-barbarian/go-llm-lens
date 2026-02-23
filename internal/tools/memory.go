package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type MemoryStore struct {
	mu   sync.RWMutex
	root string
}

func NewMemoryStore(root string) *MemoryStore {
	return &MemoryStore{root: root}
}

func (ms *MemoryStore) path() string {
	return filepath.Join(ms.root, ".llm-lens", "memories.json")
}

func (ms *MemoryStore) load() (map[string]string, error) {
	data, err := os.ReadFile(ms.path())
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading memories: %w", err)
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing memories: %w", err)
	}
	return m, nil
}

func (ms *MemoryStore) save(m map[string]string) error {
	p := ms.path()
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return fmt.Errorf("creating memories dir: %w", err)
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding memories: %w", err)
	}
	if err := os.WriteFile(p, data, 0o600); err != nil {
		return fmt.Errorf("writing memories: %w", err)
	}
	return nil
}

func (ms *MemoryStore) writeHandler() server.ToolHandlerFunc {
	return func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := req.RequireString("key")
		if err != nil {
			return nil, err
		}
		value, err := req.RequireString("value")
		if err != nil {
			return nil, err
		}
		ms.mu.Lock()
		defer ms.mu.Unlock()
		m, err := ms.load()
		if err != nil {
			return nil, err
		}
		m[key] = value
		if err := ms.save(m); err != nil {
			return nil, err
		}
		return mcp.NewToolResultText("ok"), nil
	}
}

func (ms *MemoryStore) readHandler() server.ToolHandlerFunc {
	return func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := req.RequireString("key")
		if err != nil {
			return nil, err
		}
		ms.mu.RLock()
		defer ms.mu.RUnlock()
		m, err := ms.load()
		if err != nil {
			return nil, err
		}
		v, ok := m[key]
		if !ok {
			return nil, fmt.Errorf("memory %q not found", key)
		}
		return mcp.NewToolResultText(v), nil
	}
}

func (ms *MemoryStore) listHandler() server.ToolHandlerFunc {
	return func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ms.mu.RLock()
		defer ms.mu.RUnlock()
		m, err := ms.load()
		if err != nil {
			return nil, err
		}
		return jsonResult(m)
	}
}

func (ms *MemoryStore) deleteHandler() server.ToolHandlerFunc {
	return func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := req.RequireString("key")
		if err != nil {
			return nil, err
		}
		ms.mu.Lock()
		defer ms.mu.Unlock()
		m, err := ms.load()
		if err != nil {
			return nil, err
		}
		if _, ok := m[key]; !ok {
			return nil, fmt.Errorf("memory %q not found", key)
		}
		delete(m, key)
		if err := ms.save(m); err != nil {
			return nil, err
		}
		return mcp.NewToolResultText("ok"), nil
	}
}
