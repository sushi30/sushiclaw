package email

import (
	"testing"
)

func TestThreadManager_BasicParentLink(t *testing.T) {
	tm := NewThreadManager()
	tm.ProcessHeaders("root@x.com", "Hello", "", "")
	tm.ProcessHeaders("child@x.com", "Re: Hello", "root@x.com", "<root@x.com>")

	child, ok := tm.AllMessages["child@x.com"]
	if !ok {
		t.Fatal("child not in AllMessages")
	}
	if child.Parent == nil {
		t.Fatal("child.Parent is nil")
	}
	if child.Parent.MessageID != "root@x.com" {
		t.Errorf("parent ID = %q, want root@x.com", child.Parent.MessageID)
	}

	root := tm.AllMessages["root@x.com"]
	if len(root.Children) != 1 || root.Children[0].MessageID != "child@x.com" {
		t.Errorf("root.Children = %v", root.Children)
	}
}

func TestThreadManager_GhostNode(t *testing.T) {
	tm := NewThreadManager()
	// Child arrives before parent.
	tm.ProcessHeaders("child@x.com", "Re: Missing", "missing-parent@x.com", "<missing-parent@x.com>")

	ghost, ok := tm.AllMessages["missing-parent@x.com"]
	if !ok {
		t.Fatal("ghost node not created")
	}
	if !ghost.IsGhost {
		t.Error("expected IsGhost=true")
	}
	if len(ghost.Children) != 1 || ghost.Children[0].MessageID != "child@x.com" {
		t.Errorf("ghost.Children = %v", ghost.Children)
	}

	// Parent arrives later — ghost should be promoted.
	tm.ProcessHeaders("missing-parent@x.com", "Original", "", "")
	if ghost.IsGhost {
		t.Error("IsGhost should be cleared after real message arrives")
	}
	if ghost.Subject != "Original" {
		t.Errorf("Subject = %q, want Original", ghost.Subject)
	}
}

func TestThreadManager_DuplicateChildGuard(t *testing.T) {
	tm := NewThreadManager()
	tm.ProcessHeaders("root@x.com", "Hello", "", "")
	tm.ProcessHeaders("child@x.com", "Re: Hello", "root@x.com", "<root@x.com>")
	tm.ProcessHeaders("child@x.com", "Re: Hello", "root@x.com", "<root@x.com>") // duplicate

	root := tm.AllMessages["root@x.com"]
	if len(root.Children) != 1 {
		t.Errorf("expected 1 child, got %d", len(root.Children))
	}
}

func TestThreadManager_CircularReferenceGuard(t *testing.T) {
	tm := NewThreadManager()
	// Self-referencing In-Reply-To should not self-link.
	tm.ProcessHeaders("self@x.com", "Loop", "self@x.com", "<self@x.com>")

	node := tm.AllMessages["self@x.com"]
	if node.Parent != nil {
		t.Errorf("self-referencing node should have nil Parent, got %v", node.Parent)
	}
	if len(node.Children) != 0 {
		t.Errorf("self-referencing node should have no children, got %d", len(node.Children))
	}
}

func TestThreadManager_ReferencesChain(t *testing.T) {
	tm := NewThreadManager()
	tm.ProcessHeaders("msg1@x.com", "Root", "", "")
	tm.ProcessHeaders("msg2@x.com", "Re: Root", "msg1@x.com", "<msg1@x.com>")
	tm.ProcessHeaders("msg3@x.com", "Re: Root", "msg2@x.com", "<msg1@x.com> <msg2@x.com>")

	chain := tm.ReferencesChain("msg3@x.com")
	want := []string{"msg1@x.com", "msg2@x.com"}
	if len(chain) != len(want) {
		t.Fatalf("ReferencesChain = %v, want %v", chain, want)
	}
	for i, id := range want {
		if chain[i] != id {
			t.Errorf("chain[%d] = %q, want %q", i, chain[i], id)
		}
	}
}

func TestThreadManager_ThreadID(t *testing.T) {
	tm := NewThreadManager()
	tm.ProcessHeaders("root@x.com", "Root", "", "")
	tm.ProcessHeaders("child@x.com", "Re: Root", "root@x.com", "<root@x.com>")
	tm.ProcessHeaders("grandchild@x.com", "Re: Root", "child@x.com", "<root@x.com> <child@x.com>")

	if got := tm.ThreadID("root@x.com"); got != "root@x.com" {
		t.Errorf("ThreadID(root) = %q, want root@x.com", got)
	}
	if got := tm.ThreadID("child@x.com"); got != "root@x.com" {
		t.Errorf("ThreadID(child) = %q, want root@x.com", got)
	}
	if got := tm.ThreadID("grandchild@x.com"); got != "root@x.com" {
		t.Errorf("ThreadID(grandchild) = %q, want root@x.com", got)
	}
	if got := tm.ThreadID("unknown@x.com"); got != "unknown@x.com" {
		t.Errorf("ThreadID(unknown) = %q, want unknown@x.com", got)
	}
}

func TestThreadManager_BuildThreads(t *testing.T) {
	tm := NewThreadManager()
	tm.ProcessHeaders("root1@x.com", "Thread A", "", "")
	tm.ProcessHeaders("root2@x.com", "Thread B", "", "")
	tm.ProcessHeaders("child@x.com", "Re: Thread A", "root1@x.com", "<root1@x.com>")
	tm.BuildThreads()

	if len(tm.Threads) != 2 {
		t.Errorf("expected 2 root threads, got %d", len(tm.Threads))
	}
}

func TestCleanSubject(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Hello", "Hello"},
		{"Re: Hello", "Hello"},
		{"re: Hello", "Hello"},
		{"RE: Hello", "Hello"},
		{"Fwd: Hello", "Hello"},
		{"fwd: Hello", "Hello"},
		{"Re: Fwd: Hello", "Hello"},
		{"Re: Re: Hello", "Hello"},
		{"Re:Hello", "Hello"}, // no space after colon
		{"Fwd:Hello", "Hello"},
		{"  Re:  Hello  ", "Hello"},
		{"Hello World", "Hello World"},
	}
	for _, c := range cases {
		got := cleanSubject(c.in)
		if got != c.want {
			t.Errorf("cleanSubject(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCleanID(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"<foo@bar.com>", "foo@bar.com"},
		{"foo@bar.com", "foo@bar.com"},
		{"  <foo@bar.com>  ", "foo@bar.com"},
		{"", ""},
	}
	for _, c := range cases {
		got := cleanID(c.in)
		if got != c.want {
			t.Errorf("cleanID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
