package fs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadFileTool_ReadsContent(t *testing.T) {
	dir := t.TempDir()
	requireWriteFile(t, filepath.Join(dir, "notes.txt"), "hello world")

	tool := NewReadFileTool(dir, true, 64)
	out, err := tool.Execute(context.Background(), `{"path":"notes.txt"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !strings.Contains(out, "hello world") {
		t.Fatalf("output = %q, want file content", out)
	}
	if !strings.Contains(out, "[END OF FILE") {
		t.Fatalf("output = %q, want EOF marker", out)
	}
}

func TestReadFileTool_PaginatesByOffsetAndLength(t *testing.T) {
	dir := t.TempDir()
	requireWriteFile(t, filepath.Join(dir, "long.txt"), "abcdef")

	tool := NewReadFileTool(dir, true, 4)
	out, err := tool.Execute(context.Background(), `{"path":"long.txt","offset":1,"length":3}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !strings.Contains(out, "bcd") {
		t.Fatalf("output = %q, want requested slice", out)
	}
	if !strings.Contains(out, "TRUNCATED") {
		t.Fatalf("output = %q, want truncation marker", out)
	}
}

func TestReadFileTool_BlocksWorkspaceEscape(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	requireWriteFile(t, filepath.Join(outside, "secret.txt"), "secret")

	tool := NewReadFileTool(dir, true, 64)
	_, err := tool.Execute(context.Background(), `{"path":"../secret.txt"}`)
	if err == nil {
		t.Fatal("expected workspace escape error")
	}
	if !strings.Contains(err.Error(), "outside the workspace") && !strings.Contains(err.Error(), "escapes workspace") {
		t.Fatalf("error = %v, want workspace denial", err)
	}
}

func TestReadFileTool_BlocksSymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	requireWriteFile(t, filepath.Join(outside, "secret.txt"), "secret")
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(dir, "link.txt")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	tool := NewReadFileTool(dir, true, 64)
	_, err := tool.Execute(context.Background(), `{"path":"link.txt"}`)
	if err == nil {
		t.Fatal("expected symlink escape error")
	}
	if !strings.Contains(err.Error(), "access denied") && !strings.Contains(err.Error(), "outside the workspace") {
		t.Fatalf("error = %v, want symlink denial", err)
	}
}

func TestWriteFileTool_WritesNewFileAndCreatesParents(t *testing.T) {
	dir := t.TempDir()
	tool := NewWriteFileTool(dir, true)

	out, err := tool.Execute(context.Background(), `{"path":"nested/out.txt","content":"hello\r\nworld"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "File written") {
		t.Fatalf("output = %q, want success message", out)
	}

	got, err := os.ReadFile(filepath.Join(dir, "nested", "out.txt"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "hello\r\nworld" {
		t.Fatalf("content = %q, want CRLF-preserving content", string(got))
	}
}

func TestWriteFileTool_RequiresOverwriteForExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.txt")
	requireWriteFile(t, path, "old")
	tool := NewWriteFileTool(dir, true)

	_, err := tool.Execute(context.Background(), `{"path":"existing.txt","content":"new"}`)
	if err == nil {
		t.Fatal("expected overwrite error")
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "old" {
		t.Fatalf("content = %q, want original content", string(got))
	}

	_, err = tool.Execute(context.Background(), `{"path":"existing.txt","content":"new","overwrite":true}`)
	if err != nil {
		t.Fatalf("Execute overwrite: %v", err)
	}
	got, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile after overwrite: %v", err)
	}
	if string(got) != "new" {
		t.Fatalf("content = %q, want overwritten content", string(got))
	}
}

func TestListDirTool_ListsEntries(t *testing.T) {
	dir := t.TempDir()
	requireWriteFile(t, filepath.Join(dir, "file.txt"), "content")
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	tool := NewListDirTool(dir, true)
	out, err := tool.Execute(context.Background(), `{}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !strings.Contains(out, "FILE: file.txt") {
		t.Fatalf("output = %q, want file entry", out)
	}
	if !strings.Contains(out, "DIR:  subdir") {
		t.Fatalf("output = %q, want directory entry", out)
	}
}

func TestToolsExposeExpectedMetadata(t *testing.T) {
	read := NewReadFileTool("", false, 64)
	write := NewWriteFileTool("", false)
	list := NewListDirTool("", false)

	if read.Name() != "read_file" || write.Name() != "write_file" || list.Name() != "list_dir" {
		t.Fatalf("unexpected names: %q %q %q", read.Name(), write.Name(), list.Name())
	}
	if !read.Parameters()["path"].Required {
		t.Fatal("read_file path should be required")
	}
	if !write.Parameters()["path"].Required || !write.Parameters()["content"].Required {
		t.Fatal("write_file path and content should be required")
	}
	if list.Parameters()["path"].Required {
		t.Fatal("list_dir path should be optional")
	}
}

func requireWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}
