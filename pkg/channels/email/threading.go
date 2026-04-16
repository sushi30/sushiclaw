package email

import "strings"

// MessageNode represents a single email in a thread tree.
type MessageNode struct {
	MessageID string
	Subject   string
	Children  []*MessageNode
	Parent    *MessageNode
	// IsGhost is true when the node was created to represent a referenced parent
	// that has not been fetched yet. It is cleared when the real message arrives.
	IsGhost bool
}

// ThreadManager groups emails into hierarchical conversation trees using the
// JWZ threading algorithm (RFC 5322 Section 3.6.4).
type ThreadManager struct {
	// AllMessages is a global lookup table keyed by raw Message-ID (no angle brackets).
	AllMessages map[string]*MessageNode
	// Threads contains only the root nodes (messages with no known parent).
	Threads []*MessageNode
}

// NewThreadManager initialises an empty threading engine.
func NewThreadManager() *ThreadManager {
	return &ThreadManager{
		AllMessages: make(map[string]*MessageNode),
	}
}

// ProcessHeaders parses standard RFC 5322 threading headers and links the
// message into the conversation tree.
//
// msgID     — the message's own Message-ID (angle brackets stripped by caller or here)
// subject   — raw Subject header value
// inReplyTo — single Message-ID from the In-Reply-To header (or "")
// references — space-separated Message-IDs from the References header (or "")
func (tm *ThreadManager) ProcessHeaders(msgID, subject, inReplyTo, references string) {
	msgID = cleanID(msgID)
	if msgID == "" {
		return
	}

	// Get or create this node.
	node, exists := tm.AllMessages[msgID]
	if !exists {
		node = &MessageNode{MessageID: msgID}
		tm.AllMessages[msgID] = node
	}
	node.Subject = cleanSubject(subject)
	node.IsGhost = false

	// Identify the immediate parent.
	// References list is more comprehensive; its last element is the direct parent.
	var parentID string
	refs := strings.Fields(references)
	if len(refs) > 0 {
		parentID = cleanID(refs[len(refs)-1])
	} else if inReplyTo != "" {
		parentID = cleanID(inReplyTo)
	}

	if parentID == "" || parentID == msgID {
		return
	}

	parentNode, pExists := tm.AllMessages[parentID]
	if !pExists {
		// Create a ghost placeholder so the child is linked even when the
		// parent message hasn't been fetched yet.
		parentNode = &MessageNode{MessageID: parentID, IsGhost: true}
		tm.AllMessages[parentID] = parentNode
	}

	// Guard against duplicate children and circular references.
	for _, child := range parentNode.Children {
		if child.MessageID == msgID {
			return
		}
	}

	node.Parent = parentNode
	parentNode.Children = append(parentNode.Children, node)
}

// BuildThreads rebuilds the Threads slice from AllMessages.
// Call after all ProcessHeaders calls are done if you need the root list.
func (tm *ThreadManager) BuildThreads() {
	tm.Threads = tm.Threads[:0]
	for _, node := range tm.AllMessages {
		if node.Parent == nil {
			tm.Threads = append(tm.Threads, node)
		}
	}
}

// ReferencesChain returns the ordered Message-IDs (no angle brackets) from the
// thread root down to msgID's parent, suitable for the RFC 5322 References header.
func (tm *ThreadManager) ReferencesChain(msgID string) []string {
	node, ok := tm.AllMessages[msgID]
	if !ok {
		return nil
	}

	// Walk up the parent chain, collecting IDs.
	var chain []string
	cur := node.Parent
	for cur != nil {
		chain = append(chain, cur.MessageID)
		cur = cur.Parent
	}

	// Reverse so oldest ancestor is first.
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain
}

// cleanID strips angle brackets and surrounding whitespace from a Message-ID.
func cleanID(id string) string {
	return strings.Trim(id, "<> ")
}

// cleanSubject strips all Re: and Fwd: prefixes (case-insensitive) while
// preserving the original case of the remaining subject text. This allows
// reply subjects to be matched back to the thread root and used verbatim
// when constructing "Re: <subject>" headers.
func cleanSubject(sub string) string {
	s := strings.TrimSpace(sub)
	for {
		lower := strings.ToLower(s)
		switch {
		case strings.HasPrefix(lower, "re:"):
			s = strings.TrimSpace(s[3:])
		case strings.HasPrefix(lower, "fwd:"):
			s = strings.TrimSpace(s[4:])
		default:
			return s
		}
	}
}
