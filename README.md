# go-llm-lens

[![CI](https://github.com/tender-barbarian/go-llm-lens/actions/workflows/ci.yml/badge.svg)](https://github.com/tender-barbarian/go-llm-lens/actions/workflows/ci.yml)
[![Release](https://github.com/tender-barbarian/go-llm-lens/actions/workflows/release.yml/badge.svg)](https://github.com/tender-barbarian/go-llm-lens/releases/latest)

An MCP (Model Context Protocol) server that enables LLMs like Claude to navigate and understand Go codebases through full type-checked AST analysis.

Instead of reading raw source files line by line, an LLM can call structured tools to explore packages, find symbols, inspect function signatures and type definitions, and discover interface implementations.

## Prerequisites

- Go 1.25+
- The target codebase must compile cleanly (`go build ./...` passes)
- Dependencies must be downloaded (`go mod download` has been run)

## Installation

**Download a pre-built binary** from the [latest release](https://github.com/tender-barbarian/go-llm-lens/releases/latest) for your platform (Linux, macOS, Windows — amd64 and arm64).

**Or install with Go:**

```bash
go install github.com/tender-barbarian/go-llm-lens/cmd/server@latest
```

**Or build from source:**

```bash
git clone https://github.com/tender-barbarian/go-llm-lens
cd go-llm-lens
go build -o go-llm-lens ./cmd/server
```

## Usage

```bash
go-llm-lens --root /path/to/your/go/repo
```

The server communicates over **stdio** using the MCP protocol.

### Flags

| Flag     | Default | Description                                |
|----------|---------|--------------------------------------------|
| `--root` | `.`     | Root directory of the Go codebase to index |

### Add to Claude Code

```
claude mcp add --scope user --transport stdio go-llm-lens -- /path/to/go-llm-lens --root /path/to/your/go/repo
```

## MCP Tools

All tools return JSON-encoded results.

### `list_packages`

Lists all indexed packages with summary statistics.

| Field    | Type   | Required | Description                          |
|----------|--------|----------|--------------------------------------|
| `filter` | string | no       | Optional prefix filter on import path |

**Output:** Array of `{ import_path, name, dir, file_count, func_count, type_count }`

### `get_package_symbols`

Returns all symbols in a package: functions, types, variables, and constants.

| Field                | Type   | Required | Description                                    |
|----------------------|--------|----------|------------------------------------------------|
| `package`            | string | yes      | Package import path                            |
| `include_unexported` | bool   | no       | Include unexported symbols (default: false)    |

**Output:** `{ funcs: [...], types: [...], vars: [...] }` each with signature and doc comment.

### `find_symbol`

Searches for a symbol by name across the entire indexed codebase.

| Field  | Type   | Required | Description                                                          |
|--------|--------|----------|----------------------------------------------------------------------|
| `name` | string | yes      | Symbol name (exact match)                                            |
| `kind` | string | no       | Filter by kind: `func`, `method`, `type`, `var`, `const` (empty = all) |

**Output:** Array of matches with package, kind, signature, location.

### `get_function`

Returns full details for a specific function or method.

| Field     | Type   | Required | Description                                     |
|-----------|--------|----------|-------------------------------------------------|
| `package` | string | yes      | Package import path                             |
| `name`    | string | yes      | Function name, or `TypeName.MethodName` for methods |

**Output:** Full signature, parameter names and types, return types, doc comment, file and line.

### `get_type`

Returns full definition of a type (struct or interface).

| Field     | Type   | Required | Description         |
|-----------|--------|----------|---------------------|
| `package` | string | yes      | Package import path |
| `name`    | string | yes      | Type name           |

**Output:**
- For structs: fields with types, struct tags, and comments; all methods; embedded types
- For interfaces: method signatures with parameter and return types; embedded interfaces
- Doc comment, file and line

### `find_implementations`

Finds all concrete types in the indexed codebase that implement a given interface.

| Field       | Type   | Required | Description                              |
|-------------|--------|----------|------------------------------------------|
| `package`   | string | yes      | Package import path of the interface     |
| `interface` | string | yes      | Interface type name                      |

**Output:** Array of `{ name, package, location, implements_via }` where `implements_via` is `"value"` or `"pointer"`.

Uses `types.Implements` from `go/types` for precise, type-system-accurate results.

## How it works

The indexer uses `golang.org/x/tools/go/packages` to perform full type-checked loading of the entire codebase at startup, then builds an in-memory index of all packages, functions, types, variables, and constants. The index is queried by MCP tools without re-parsing source files.

## Limitations

- **Codebase must build.** Full type checking requires the code to compile. Broken packages are skipped with a warning.
- **Dependencies must be available.** Run `go mod download` in the target codebase before starting the server.
- **Index is built at startup.** Changes to the codebase require restarting the server.
- **One codebase per server instance.** Use multiple server instances for multiple codebases.
- **Standard library not indexed.** Only packages under the module root (`./...`) are indexed.

## License

See [LICENSE](LICENSE).
