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

func TestReadFileTool_ReadsLineRange(t *testing.T) {
	dir := t.TempDir()
	requireWriteFile(t, filepath.Join(dir, "notes.txt"), "one\ntwo\nthree\nfour\n")

	tool := NewReadFileTool(dir, true, 64)
	out, err := tool.Execute(context.Background(), `{"path":"notes.txt","start":2,"end":3}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !strings.Contains(out, "read: lines 2-3") {
		t.Fatalf("output = %q, want line range header", out)
	}
	if !strings.Contains(out, "two\nthree\n") {
		t.Fatalf("output = %q, want requested lines", out)
	}
	if strings.Contains(out, "one") || strings.Contains(out, "four") {
		t.Fatalf("output = %q, want only requested line range", out)
	}
	if !strings.Contains(out, "TRUNCATED") {
		t.Fatalf("output = %q, want truncation marker after selected range", out)
	}
}

func TestReadFileTool_ReadsFromStartLineToEOF(t *testing.T) {
	dir := t.TempDir()
	requireWriteFile(t, filepath.Join(dir, "notes.txt"), "one\ntwo\nthree\n")

	tool := NewReadFileTool(dir, true, 64)
	out, err := tool.Execute(context.Background(), `{"path":"notes.txt","start":2}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !strings.Contains(out, "read: lines 2-3") {
		t.Fatalf("output = %q, want actual line range header", out)
	}
	if !strings.Contains(out, "two\nthree\n") {
		t.Fatalf("output = %q, want content from start line to EOF", out)
	}
	if strings.Contains(out, "one") {
		t.Fatalf("output = %q, want content before start omitted", out)
	}
	if !strings.Contains(out, "[END OF FILE") {
		t.Fatalf("output = %q, want EOF marker", out)
	}
}

func TestReadFileTool_RejectsInvalidLineRanges(t *testing.T) {
	dir := t.TempDir()
	requireWriteFile(t, filepath.Join(dir, "notes.txt"), "one\ntwo\n")

	tests := []struct {
		name string
		args string
		want string
	}{
		{
			name: "start less than one",
			args: `{"path":"notes.txt","start":0,"end":1}`,
			want: "start must be >= 1",
		},
		{
			name: "end before start",
			args: `{"path":"notes.txt","start":2,"end":1}`,
			want: "end must be >= start",
		},
		{
			name: "zero end",
			args: `{"path":"notes.txt","end":0}`,
			want: "end must be >= start",
		},
		{
			name: "mixed byte and line pagination",
			args: `{"path":"notes.txt","start":1,"offset":1}`,
			want: "line range cannot be combined with offset or length",
		},
	}

	tool := NewReadFileTool(dir, true, 64)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tool.Execute(context.Background(), tt.args)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
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
	if read.Parameters()["start"].Required || read.Parameters()["end"].Required {
		t.Fatal("read_file start and end should be optional")
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
