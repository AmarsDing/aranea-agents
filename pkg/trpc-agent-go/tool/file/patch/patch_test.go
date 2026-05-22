//
// Tencent is pleased to support the open source community by making
// trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//

package patch_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"trpc.group/trpc-go/trpc-agent-go/tool/file/patch"
)

func TestApplyEditsSingle(t *testing.T) {
	out, n, applied, err := patch.ApplyEdits("alpha\nbeta\n", []patch.Edit{
		{Search: "beta", Replace: "gamma"},
	})
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, 1, applied)
	require.Equal(t, "alpha\ngamma\n", out)
}

func TestApplyEditsSkipsNoOp(t *testing.T) {
	out, n, applied, err := patch.ApplyEdits("x\n", []patch.Edit{
		{Search: "x", Replace: "x"},
		{Search: "x", Replace: "y"},
	})
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, 1, applied)
	require.Equal(t, "y\n", out)
}

func TestApplyEditsNotUnique(t *testing.T) {
	_, _, _, err := patch.ApplyEdits("foo\nfoo\n", []patch.Edit{
		{Search: "foo", Replace: "bar"},
	})
	require.Error(t, err)
	var nu *patch.EditNotUniqueError
	require.ErrorAs(t, err, &nu)
}

func TestApplyHunks(t *testing.T) {
	content := "line1\nline2\nline3\n"
	hunks := []patch.Hunk{{
		OldStart: 2,
		OldLines: 1,
		NewStart: 2,
		NewLines: 1,
		Lines:    []string{"-line2", "+LINE2"},
	}}
	out, err := patch.ApplyHunks(content, hunks)
	require.NoError(t, err)
	require.Contains(t, out, "LINE2")
	require.NotContains(t, out, "line2")
}

func TestParseUnifiedAndApply(t *testing.T) {
	content := "func foo() {\n    return 1\n}\n"
	patchText := strings.Join([]string{
		"@@ -1,3 +1,3 @@",
		" func foo() {",
		"-    return 1",
		"+    return 2",
		" }",
	}, "\n")
	hunks, err := patch.ParseUnified(patchText)
	require.NoError(t, err)
	out, err := patch.ApplyHunks(content, hunks)
	require.NoError(t, err)
	require.Contains(t, out, "return 2")
}

func TestApplyHunksMismatch(t *testing.T) {
	content := "a\nb\nc\n"
	hunks := []patch.Hunk{{
		OldStart: 2,
		OldLines: 1,
		NewStart: 2,
		NewLines: 1,
		Lines:    []string{"-wrong"},
	}}
	_, err := patch.ApplyHunks(content, hunks)
	require.Error(t, err)
}

func TestApplyHunksMultiple(t *testing.T) {
	content := "a\nb\nc\nd\n"
	hunks := []patch.Hunk{
		{
			OldStart: 1,
			OldLines: 1,
			NewStart: 1,
			NewLines: 1,
			Lines:    []string{"-a", "+A"},
		},
		{
			OldStart: 3,
			OldLines: 1,
			NewStart: 3,
			NewLines: 1,
			Lines:    []string{"-c", "+C"},
		},
	}
	out, err := patch.ApplyHunks(content, hunks)
	require.NoError(t, err)
	require.Equal(t, "A\nb\nC\nd\n", out)
}
