package realworld

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var filesystemManifest = serverManifest{
	Name:           "filesystem",
	GatewayID:      "filesystem",
	ServerID:       "filesystem",
	CommandEnvVar:  "CENTIAN_FILESYSTEM_SERVER_CMD",
	ArgsEnvVar:     "CENTIAN_FILESYSTEM_SERVER_ARGS",
	DefaultCommand: "npx",
	DefaultArgs:    []string{"-y", "@modelcontextprotocol/server-filesystem"},
	ExpectedTools: []string{
		"create_directory",
		"directory_tree",
		"edit_file",
		"get_file_info",
		"list_allowed_directories",
		"list_directory",
		"list_directory_with_sizes",
		"move_file",
		"read_media_file",
		"read_multiple_files",
		"read_text_file",
		"search_files",
		"write_file",
	},
	BuildFixture: buildFilesystemFixture,
	Normalize:    normalizeFilesystemResult,
}

func TestFilesystemToolCatalogParity(t *testing.T) {
	runServerComparison(t, filesystemManifest, "tool_catalog", func(ctx context.Context, t *testing.T, pair *connectionPair, _ *fixtureBundle) {
		assertToolCatalogParity(ctx, t, filesystemManifest, pair)
	})
}

func TestFilesystemListAllowedDirectoriesParity(t *testing.T) {
	runServerComparison(t, filesystemManifest, "list_allowed_directories", func(ctx context.Context, t *testing.T, pair *connectionPair, fixture *fixtureBundle) {
		assertToolCallParity(ctx, t, filesystemManifest, fixture, pair, "list_allowed_directories", map[string]any{}, map[string]any{})
	})
}

func TestFilesystemListDirectoryParity(t *testing.T) {
	runServerComparison(t, filesystemManifest, "list_directory", func(ctx context.Context, t *testing.T, pair *connectionPair, fixture *fixtureBundle) {
		assertToolCallParity(
			ctx,
			t,
			filesystemManifest,
			fixture,
			pair,
			"list_directory",
			map[string]any{"path": fixture.Direct.RootDir},
			map[string]any{"path": fixture.Proxied.RootDir},
		)
	})
}

func TestFilesystemDirectoryTreeParity(t *testing.T) {
	runServerComparison(t, filesystemManifest, "directory_tree", func(ctx context.Context, t *testing.T, pair *connectionPair, fixture *fixtureBundle) {
		assertToolCallParity(
			ctx,
			t,
			filesystemManifest,
			fixture,
			pair,
			"directory_tree",
			map[string]any{"path": fixture.Direct.RootDir},
			map[string]any{"path": fixture.Proxied.RootDir},
		)
	})
}

func TestFilesystemReadTextFileParity(t *testing.T) {
	runServerComparison(t, filesystemManifest, "read_text_file", func(ctx context.Context, t *testing.T, pair *connectionPair, fixture *fixtureBundle) {
		readRel := fixture.Expected["read_rel"]
		assertToolCallParity(
			ctx,
			t,
			filesystemManifest,
			fixture,
			pair,
			"read_text_file",
			map[string]any{"path": modePath(fixture.Direct, readRel)},
			map[string]any{"path": modePath(fixture.Proxied, readRel)},
		)
	})
}

func TestFilesystemSearchFilesParity(t *testing.T) {
	runServerComparison(t, filesystemManifest, "search_files", func(ctx context.Context, t *testing.T, pair *connectionPair, fixture *fixtureBundle) {
		assertToolCallParity(
			ctx,
			t,
			filesystemManifest,
			fixture,
			pair,
			"search_files",
			map[string]any{
				"path":    fixture.Direct.RootDir,
				"pattern": fixture.Expected["search_pattern"],
			},
			map[string]any{
				"path":    fixture.Proxied.RootDir,
				"pattern": fixture.Expected["search_pattern"],
			},
		)
	})
}

func TestFilesystemGetFileInfoParity(t *testing.T) {
	runServerComparison(t, filesystemManifest, "get_file_info", func(ctx context.Context, t *testing.T, pair *connectionPair, fixture *fixtureBundle) {
		infoRel := fixture.Expected["info_rel"]
		assertToolCallParity(
			ctx,
			t,
			filesystemManifest,
			fixture,
			pair,
			"get_file_info",
			map[string]any{"path": modePath(fixture.Direct, infoRel)},
			map[string]any{"path": modePath(fixture.Proxied, infoRel)},
		)
	})
}

func TestFilesystemCreateDirectoryParity(t *testing.T) {
	runServerComparison(t, filesystemManifest, "create_directory", func(ctx context.Context, t *testing.T, pair *connectionPair, fixture *fixtureBundle) {
		targetRel := fixture.Expected["create_dir_rel"]
		assertToolCallParity(
			ctx,
			t,
			filesystemManifest,
			fixture,
			pair,
			"create_directory",
			map[string]any{"path": modePath(fixture.Direct, targetRel)},
			map[string]any{"path": modePath(fixture.Proxied, targetRel)},
		)
	})
}

func TestFilesystemWriteFileParity(t *testing.T) {
	runServerComparison(t, filesystemManifest, "write_file", func(ctx context.Context, t *testing.T, pair *connectionPair, fixture *fixtureBundle) {
		targetRel := fixture.Expected["write_rel"]
		content := fixture.Expected["write_content"]
		assertToolCallParity(
			ctx,
			t,
			filesystemManifest,
			fixture,
			pair,
			"write_file",
			map[string]any{"path": modePath(fixture.Direct, targetRel), "content": content},
			map[string]any{"path": modePath(fixture.Proxied, targetRel), "content": content},
		)
	})
}

func TestFilesystemEditFileDryRunParity(t *testing.T) {
	runServerComparison(t, filesystemManifest, "edit_file_dry_run", func(ctx context.Context, t *testing.T, pair *connectionPair, fixture *fixtureBundle) {
		editRel := fixture.Expected["edit_rel"]
		oldText := fixture.Expected["edit_old"]
		newText := fixture.Expected["edit_new"]

		assertToolCallParity(
			ctx,
			t,
			filesystemManifest,
			fixture,
			pair,
			"edit_file",
			map[string]any{
				"path":   modePath(fixture.Direct, editRel),
				"edits":  []map[string]any{{"oldText": oldText, "newText": newText}},
				"dryRun": true,
			},
			map[string]any{
				"path":   modePath(fixture.Proxied, editRel),
				"edits":  []map[string]any{{"oldText": oldText, "newText": newText}},
				"dryRun": true,
			},
		)
	})
}

func TestFilesystemMoveFileParity(t *testing.T) {
	runServerComparison(t, filesystemManifest, "move_file", func(ctx context.Context, t *testing.T, pair *connectionPair, fixture *fixtureBundle) {
		sourceRel := fixture.Expected["move_source_rel"]
		destRel := fixture.Expected["move_dest_rel"]
		assertToolCallParity(
			ctx,
			t,
			filesystemManifest,
			fixture,
			pair,
			"move_file",
			map[string]any{
				"source":      modePath(fixture.Direct, sourceRel),
				"destination": modePath(fixture.Direct, destRel),
			},
			map[string]any{
				"source":      modePath(fixture.Proxied, sourceRel),
				"destination": modePath(fixture.Proxied, destRel),
			},
		)
	})
}

func TestFilesystemRootsUpdateParity(t *testing.T) {
	runServerComparison(t, filesystemManifest, "roots_update", func(ctx context.Context, t *testing.T, pair *connectionPair, fixture *fixtureBundle) {
		directExtraRoot := modePath(fixture.Direct, fixture.Expected["extra_root_rel"])
		proxiedExtraRoot := modePath(fixture.Proxied, fixture.Expected["extra_root_rel"])

		pair.Direct.client.AddRoots(&mcp.Root{Name: "filesystem-extra", URI: fileURI(directExtraRoot)})
		pair.Proxied.client.AddRoots(&mcp.Root{Name: "filesystem-extra", URI: fileURI(proxiedExtraRoot)})

		directResult, directErr := waitForCallResult(ctx, pair.Direct.session, "list_allowed_directories", map[string]any{}, func(result *mcp.CallToolResult, err error) bool {
			if err != nil || result == nil {
				return false
			}
			return strings.Contains(prettyJSON(t, normalizeResultForComparison(t, filesystemManifest, modeDirect, fixture, result)), "/__EXTRA_ROOT__")
		})
		proxiedResult, proxiedErr := waitForCallResult(ctx, pair.Proxied.session, "list_allowed_directories", map[string]any{}, func(result *mcp.CallToolResult, err error) bool {
			if err != nil || result == nil {
				return false
			}
			return strings.Contains(prettyJSON(t, normalizeResultForComparison(t, filesystemManifest, modeProxied, fixture, result)), "/__EXTRA_ROOT__")
		})

		assertCallOutcomeParity(t, filesystemManifest, fixture, directResult, directErr, proxiedResult, proxiedErr)
	})
}

func buildFilesystemFixture(t *testing.T) *fixtureBundle {
	t.Helper()

	baseDir := t.TempDir()
	seedDir := filepath.Join(baseDir, "seed")
	if err := os.MkdirAll(filepath.Join(seedDir, "docs"), 0o755); err != nil {
		t.Fatalf("failed to create seed docs dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(seedDir, "workspace"), 0o755); err != nil {
		t.Fatalf("failed to create seed workspace dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(seedDir, "docs", "readme.txt"), []byte("Hello from filesystem fixture.\nSecond line.\n"), 0o644); err != nil {
		t.Fatalf("failed to seed readme: %v", err)
	}
	if err := os.WriteFile(filepath.Join(seedDir, "workspace", "move_me.txt"), []byte("Move me.\n"), 0o644); err != nil {
		t.Fatalf("failed to seed move file: %v", err)
	}

	directRoot := filepath.Join(baseDir, "direct-root")
	proxiedRoot := filepath.Join(baseDir, "proxied-root")
	copyTree(t, seedDir, directRoot)
	copyTree(t, seedDir, proxiedRoot)

	if err := os.MkdirAll(filepath.Join(directRoot, "extra"), 0o755); err != nil {
		t.Fatalf("failed to create direct extra root: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(proxiedRoot, "extra"), 0o755); err != nil {
		t.Fatalf("failed to create proxied extra root: %v", err)
	}

	return &fixtureBundle{
		Direct: modeFixture{
			RootDir: directRoot,
			Roots: []*mcp.Root{
				{Name: "filesystem-root", URI: fileURI(directRoot)},
			},
			Normalization: map[string]string{
				directRoot:                                  "/__ROOT__",
				fileURI(directRoot):                         "file:///__ROOT__",
				filepath.Join(directRoot, "extra"):          "/__EXTRA_ROOT__",
				fileURI(filepath.Join(directRoot, "extra")): "file:///__EXTRA_ROOT__",
			},
		},
		Proxied: modeFixture{
			RootDir: proxiedRoot,
			Roots: []*mcp.Root{
				{Name: "filesystem-root", URI: fileURI(proxiedRoot)},
			},
			Normalization: map[string]string{
				proxiedRoot:                                  "/__ROOT__",
				fileURI(proxiedRoot):                         "file:///__ROOT__",
				filepath.Join(proxiedRoot, "extra"):          "/__EXTRA_ROOT__",
				fileURI(filepath.Join(proxiedRoot, "extra")): "file:///__EXTRA_ROOT__",
			},
		},
		Expected: map[string]string{
			"read_rel":        "docs/readme.txt",
			"info_rel":        "docs/readme.txt",
			"search_pattern":  "readme",
			"create_dir_rel":  "workspace/generated",
			"write_rel":       "workspace/new_file.txt",
			"write_content":   "Created by Centian realworld test.\n",
			"edit_rel":        "docs/readme.txt",
			"edit_old":        "Second line.",
			"edit_new":        "Updated line.",
			"move_source_rel": "workspace/move_me.txt",
			"move_dest_rel":   "workspace/moved/move_me.txt",
			"extra_root_rel":  "extra",
		},
	}
}

func normalizeFilesystemResult(mode serverMode, value any, fixture *fixtureBundle) any {
	modeFixture := fixtureForMode(fixture, mode)
	return replaceStrings(value, modeFixture.Normalization)
}
