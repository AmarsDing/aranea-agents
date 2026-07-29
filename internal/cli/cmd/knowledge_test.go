package cmd

import (
	"testing"
)

func TestNewKnowledgeCmd_Structure(t *testing.T) {
	root := NewKnowledgeCmd()
	if root.Use != "knowledge" {
		t.Errorf("Use: got %q, want %q", root.Use, "knowledge")
	}
	findCmdPath(t, root, "collections", "ls")
	get := findCmdPath(t, root, "collections", "get")
	create := findCmdPath(t, root, "collections", "create")
	del := findCmdPath(t, root, "collections", "delete")
	findCmdPath(t, root, "documents", "ls")
	docGet := findCmdPath(t, root, "documents", "get")
	docDel := findCmdPath(t, root, "documents", "delete")
	search := findCmdPath(t, root, "search")

	requireExactArgs(t, get)
	requireExactArgs(t, del)
	requireExactArgs(t, docGet)
	requireExactArgs(t, docDel)
	requireFlag(t, create, "name", true)
	requireFlag(t, create, "root-path", true)
	requireFlag(t, create, "embedding-model", false)
	requireFlag(t, search, "query", true)
}

func TestKnowledgeDocumentsLs_HasCollectionFilter(t *testing.T) {
	ls := findCmdPath(t, NewKnowledgeCmd(), "documents", "ls")
	requireFlag(t, ls, "collection-id", false)
}
