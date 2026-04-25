package fs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

const defaultMaxReadFileSize int64 = 64 * 1024

type fileToolsFS struct {
	workspace string
	restrict  bool
}

func newFileToolsFS(workspace string, restrict bool) fileToolsFS {
	return fileToolsFS{workspace: workspace, restrict: restrict}
}

func (f fileToolsFS) open(path string) (*os.File, error) {
	if !f.restrict {
		return os.Open(f.resolveHostPath(path))
	}

	root, rel, err := f.openRoot(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()

	file, err := root.Open(rel)
	if err != nil {
		return nil, formatAccessError("open file", err)
	}
	return file, nil
}

func (f fileToolsFS) writeFile(path string, content []byte) error {
	if !f.restrict {
		target := f.resolveHostPath(path)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("failed to create parent directories: %w", err)
		}
		return os.WriteFile(target, content, 0o600)
	}

	root, rel, err := f.openRoot(path)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()

	if dir := filepath.Dir(rel); dir != "." {
		if err := root.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("failed to create parent directories: %w", err)
		}
	}
	if err := root.WriteFile(rel, content, 0o600); err != nil {
		return formatAccessError("write file", err)
	}
	return nil
}

func (f fileToolsFS) exists(path string) (bool, error) {
	if !f.restrict {
		_, err := os.Stat(f.resolveHostPath(path))
		if err == nil {
			return true, nil
		}
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}

	root, rel, err := f.openRoot(path)
	if err != nil {
		return false, err
	}
	defer func() { _ = root.Close() }()

	_, err = root.Stat(rel)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, formatAccessError("stat file", err)
}

func (f fileToolsFS) readDir(path string) ([]os.DirEntry, error) {
	if path == "" {
		path = "."
	}
	if !f.restrict {
		return os.ReadDir(f.resolveHostPath(path))
	}

	root, rel, err := f.openRoot(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()

	entries, err := fs.ReadDir(root.FS(), rel)
	if err != nil {
		return nil, formatAccessError("read directory", err)
	}
	return entries, nil
}

func (f fileToolsFS) resolveHostPath(path string) string {
	if filepath.IsAbs(path) || f.workspace == "" {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(f.workspace, path))
}

func (f fileToolsFS) openRoot(path string) (*os.Root, string, error) {
	if f.workspace == "" {
		return nil, "", fmt.Errorf("workspace is not defined")
	}

	workspace, err := filepath.Abs(f.workspace)
	if err != nil {
		return nil, "", fmt.Errorf("failed to resolve workspace path: %w", err)
	}

	root, err := os.OpenRoot(workspace)
	if err != nil {
		return nil, "", fmt.Errorf("failed to open workspace: %w", err)
	}

	rel, err := safeRelPath(workspace, path)
	if err != nil {
		_ = root.Close()
		return nil, "", err
	}
	return root, rel, nil
}

func safeRelPath(workspace, path string) (string, error) {
	if path == "" {
		path = "."
	}

	cleaned := filepath.Clean(path)
	if filepath.IsAbs(cleaned) {
		rel, err := filepath.Rel(workspace, cleaned)
		if err != nil {
			return "", fmt.Errorf("failed to calculate relative path: %w", err)
		}
		cleaned = rel
	}
	if !filepath.IsLocal(cleaned) {
		return "", fmt.Errorf("access denied: path is outside the workspace")
	}
	return cleaned, nil
}

func formatAccessError(action string, err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to %s: file not found: %w", action, err)
	}
	if errors.Is(err, os.ErrPermission) ||
		strings.Contains(err.Error(), "escapes from parent") ||
		strings.Contains(err.Error(), "permission denied") ||
		strings.Contains(err.Error(), "invalid argument") {
		return fmt.Errorf("failed to %s: access denied: %w", action, err)
	}
	return fmt.Errorf("failed to %s: %w", action, err)
}

type ReadFileTool struct {
	fs      fileToolsFS
	maxSize int64
}

func NewReadFileTool(workspace string, restrict bool, maxReadFileSize int64) *ReadFileTool {
	if maxReadFileSize <= 0 {
		maxReadFileSize = defaultMaxReadFileSize
	}
	return &ReadFileTool{fs: newFileToolsFS(workspace, restrict), maxSize: maxReadFileSize}
}

func (t *ReadFileTool) Name() string { return "read_file" }

func (t *ReadFileTool) Description() string {
	return "Read the contents of a file. Supports pagination via offset and length."
}

func (t *ReadFileTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"path": {
			Type:        "string",
			Description: "Path to the file to read.",
			Required:    true,
		},
		"offset": {
			Type:        "integer",
			Description: "Byte offset to start reading from.",
			Required:    false,
		},
		"length": {
			Type:        "integer",
			Description: "Maximum number of bytes to read.",
			Required:    false,
		},
	}
}

func (t *ReadFileTool) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}

func (t *ReadFileTool) Execute(_ context.Context, args string) (string, error) {
	var req struct {
		Path   string `json:"path"`
		Offset int64  `json:"offset"`
		Length int64  `json:"length"`
	}
	if err := json.Unmarshal([]byte(args), &req); err != nil {
		return "", fmt.Errorf("invalid read_file arguments: %w", err)
	}
	if req.Path == "" {
		return "", fmt.Errorf("path is required")
	}
	if req.Offset < 0 {
		return "", fmt.Errorf("offset must be >= 0")
	}
	if req.Length <= 0 || req.Length > t.maxSize {
		req.Length = t.maxSize
	}

	file, err := t.fs.open(req.Path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("failed to stat file: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("failed to open file: path is a directory: %s", req.Path)
	}

	if _, err = file.Seek(req.Offset, io.SeekStart); err != nil {
		return "", fmt.Errorf("failed to seek to offset %d: %w", req.Offset, err)
	}

	data := make([]byte, req.Length+1)
	n, err := file.Read(data)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("failed to read file content: %w", err)
	}

	hasMore := int64(n) > req.Length
	if hasMore {
		data = data[:req.Length]
	} else {
		data = data[:n]
	}

	if len(data) == 0 {
		return "[END OF FILE - no content at this offset]", nil
	}

	end := req.Offset + int64(len(data))
	header := fmt.Sprintf("[file: %s | total: %d bytes | read: bytes %d-%d]", filepath.Base(req.Path), info.Size(), req.Offset, end-1)
	if hasMore {
		header += fmt.Sprintf("\n[TRUNCATED - file has more content. Call read_file again with offset=%d to continue.]", end)
	} else {
		header += "\n[END OF FILE - no further content.]"
	}
	return header + "\n\n" + string(data), nil
}

type WriteFileTool struct {
	fs fileToolsFS
}

func NewWriteFileTool(workspace string, restrict bool) *WriteFileTool {
	return &WriteFileTool{fs: newFileToolsFS(workspace, restrict)}
}

func (t *WriteFileTool) Name() string { return "write_file" }

func (t *WriteFileTool) Description() string {
	return "Write content to a file. If the file already exists, overwrite must be true."
}

func (t *WriteFileTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"path": {
			Type:        "string",
			Description: "Path to the file to write.",
			Required:    true,
		},
		"content": {
			Type:        "string",
			Description: "Content to write to the file.",
			Required:    true,
		},
		"overwrite": {
			Type:        "boolean",
			Description: "Set to true to overwrite an existing file.",
			Required:    false,
		},
	}
}

func (t *WriteFileTool) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}

func (t *WriteFileTool) Execute(_ context.Context, args string) (string, error) {
	var req struct {
		Path      string  `json:"path"`
		Content   *string `json:"content"`
		Overwrite bool    `json:"overwrite"`
	}
	if err := json.Unmarshal([]byte(args), &req); err != nil {
		return "", fmt.Errorf("invalid write_file arguments: %w", err)
	}
	if req.Path == "" {
		return "", fmt.Errorf("path is required")
	}
	if req.Content == nil {
		return "", fmt.Errorf("content is required")
	}

	exists, err := t.fs.exists(req.Path)
	if err != nil {
		return "", err
	}
	if exists && !req.Overwrite {
		return "", fmt.Errorf("file %s already exists. Set overwrite=true to replace", req.Path)
	}
	if err = t.fs.writeFile(req.Path, []byte(*req.Content)); err != nil {
		return "", err
	}
	return fmt.Sprintf("File written: %s", req.Path), nil
}

type ListDirTool struct {
	fs fileToolsFS
}

func NewListDirTool(workspace string, restrict bool) *ListDirTool {
	return &ListDirTool{fs: newFileToolsFS(workspace, restrict)}
}

func (t *ListDirTool) Name() string { return "list_dir" }

func (t *ListDirTool) Description() string {
	return "List files and directories in a path."
}

func (t *ListDirTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"path": {
			Type:        "string",
			Description: "Path to list. Defaults to the workspace root.",
			Required:    false,
		},
	}
}

func (t *ListDirTool) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}

func (t *ListDirTool) Execute(_ context.Context, args string) (string, error) {
	var req struct {
		Path string `json:"path"`
	}
	if strings.TrimSpace(args) != "" {
		if err := json.Unmarshal([]byte(args), &req); err != nil {
			return "", fmt.Errorf("invalid list_dir arguments: %w", err)
		}
	}
	if req.Path == "" {
		req.Path = "."
	}

	entries, err := t.fs.readDir(req.Path)
	if err != nil {
		return "", err
	}

	var out strings.Builder
	for _, entry := range entries {
		if entry.IsDir() {
			out.WriteString("DIR:  ")
		} else {
			out.WriteString("FILE: ")
		}
		out.WriteString(entry.Name())
		out.WriteByte('\n')
	}
	return out.String(), nil
}
