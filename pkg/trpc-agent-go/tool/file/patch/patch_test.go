//
// Tencent is pleased to support the open source community by making
// trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//
//

package patch

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseUnified_SingleHunk(t *testing.T) {
	text := "--- a/f.go\n" +
		"+++ b/f.go\n" +
		"@@ -1,3 +1,3 @@\n" +
		" alpha\n" +
		"-beta\n" +
		"+BETA\n" +
		" gamma\n"
	hunks, err := ParseUnified(text)
	require.NoError(t, err)
	require.Len(t, hunks, 1)
	h := hunks[0]
	assert.Equal(t, 1, h.OldStart)
	assert.Equal(t, 3, h.OldLines)
	assert.Equal(t, 1, h.NewStart)
	assert.Equal(t, 3, h.NewLines)
	assert.Equal(t,
		[]string{" alpha", "-beta", "+BETA", " gamma"},
		h.Lines,
	)
}

func TestParseUnified_MultipleHunks(t *testing.T) {
	text := "diff --git a/f.go b/f.go\n" +
		"index 123..456 100644\n" +
		"--- a/f.go\n" +
		"+++ b/f.go\n" +
		"@@ -1,2 +1,2 @@\n" +
		"-a\n" +
		"+A\n" +
		" b\n" +
		"@@ -10,2 +10,2 @@\n" +
		" c\n" +
		"-d\n" +
		"+D\n"
	hunks, err := ParseUnified(text)
	require.NoError(t, err)
	require.Len(t, hunks, 2)
	assert.Equal(t, 1, hunks[0].OldStart)
	assert.Equal(t, 10, hunks[1].OldStart)
	assert.Equal(t, []string{" c", "-d", "+D"}, hunks[1].Lines)
}

func TestParseUnified_DefaultCounts(t *testing.T) {
	hunks, err := ParseUnified("@@ -5 +5 @@\n-x\n+y\n")
	require.NoError(t, err)
	require.Len(t, hunks, 1)
	assert.Equal(t, 5, hunks[0].OldStart)
	assert.Equal(t, 1, hunks[0].OldLines)
	assert.Equal(t, 5, hunks[0].NewStart)
	assert.Equal(t, 1, hunks[0].NewLines)
}

func TestParseUnified_NewFileHunk(t *testing.T) {
	hunks, err := ParseUnified("@@ -0,0 +1,2 @@\n+one\n+two\n")
	require.NoError(t, err)
	require.Len(t, hunks, 1)
	assert.Equal(t, 0, hunks[0].OldStart)
	assert.Equal(t, 0, hunks[0].OldLines)
	assert.Equal(t, 1, hunks[0].NewStart)
	assert.Equal(t, 2, hunks[0].NewLines)
}

func TestParseUnified_NoNewlineMarkerIgnored(t *testing.T) {
	text := "@@ -1,2 +1,2 @@\n a\n-b\n+B\n\\ No newline at end of file\n"
	hunks, err := ParseUnified(text)
	require.NoError(t, err)
	require.Len(t, hunks, 1)
	assert.Equal(t, []string{" a", "-b", "+B"}, hunks[0].Lines)
}

func TestParseUnified_EmptyBodyLineIsContext(t *testing.T) {
	text := "@@ -1,3 +1,3 @@\n a\n\n b\n"
	hunks, err := ParseUnified(text)
	require.NoError(t, err)
	require.Len(t, hunks, 1)
	assert.Equal(t, []string{" a", " ", " b"}, hunks[0].Lines)
}

func TestParseUnified_CRLFNormalized(t *testing.T) {
	hunks, err := ParseUnified("@@ -1,2 +1,2 @@\r\n-a\r\n+b\r\n c\r\n")
	require.NoError(t, err)
	require.Len(t, hunks, 1)
	assert.Equal(t, []string{"-a", "+b", " c"}, hunks[0].Lines)
}

func TestParseUnified_MalformedHeader(t *testing.T) {
	_, err := ParseUnified("@@ nonsense @@\n a\n")
	require.Error(t, err)
}

func TestParseUnified_NoHunks(t *testing.T) {
	hunks, err := ParseUnified("--- a/f.go\n+++ b/f.go\n")
	require.NoError(t, err)
	assert.Empty(t, hunks)
}

func TestValidate_OK(t *testing.T) {
	h := Hunk{
		OldStart: 1, OldLines: 2,
		NewStart: 1, NewLines: 2,
		Lines: []string{" a", "-b", "+B", " c"},
	}
	// counts: old = ' ' + '-' + ' ' = 3 != 2 → adjust hunk to match.
	h.OldLines = 3
	h.NewLines = 3
	require.NoError(t, Validate([]Hunk{h}))
}

func TestValidate_CountMismatch(t *testing.T) {
	h := Hunk{
		OldStart: 1, OldLines: 5,
		NewStart: 1, NewLines: 1,
		Lines: []string{"-a", "+b"},
	}
	require.Error(t, Validate([]Hunk{h}))
}

func TestValidate_InvalidLinePrefix(t *testing.T) {
	h := Hunk{
		OldStart: 1, OldLines: 1,
		NewStart: 1, NewLines: 1,
		Lines: []string{"?bad"},
	}
	require.Error(t, Validate([]Hunk{h}))
}

func TestValidate_NegativeStart(t *testing.T) {
	h := Hunk{
		OldStart: -1, OldLines: 0,
		NewStart: 1, NewLines: 1,
		Lines: []string{"+a"},
	}
	require.Error(t, Validate([]Hunk{h}))
}

func TestApply_SingleHunk(t *testing.T) {
	content := "alpha\nbeta\ngamma\n"
	hunks := []Hunk{{
		OldStart: 1, OldLines: 3,
		NewStart: 1, NewLines: 3,
		Lines: []string{" alpha", "-beta", "+BETA", " gamma"},
	}}
	out, err := Apply(content, hunks)
	require.NoError(t, err)
	assert.Equal(t, "alpha\nBETA\ngamma\n", out)
}

func TestApply_MultipleHunksOriginalCoordinates(t *testing.T) {
	content := "a\nb\nc\nd\ne\nf\ng\nh\n"
	hunks := []Hunk{
		{
			OldStart: 1, OldLines: 2,
			NewStart: 1, NewLines: 2,
			Lines: []string{"-a", "+A", " b"},
		},
		{
			OldStart: 7, OldLines: 2,
			NewStart: 7, NewLines: 2,
			Lines: []string{" g", "-h", "+H"},
		},
	}
	out, err := Apply(content, hunks)
	require.NoError(t, err)
	assert.Equal(t, "A\nb\nc\nd\ne\nf\ng\nH\n", out)
}

func TestApply_DriftedLineNumbersUniqueMatch(t *testing.T) {
	// OldStart is wrong (says 1) but the old sequence is unique.
	content := "x\ny\nz\nalpha\nbeta\ngamma\n"
	hunks := []Hunk{{
		OldStart: 1, OldLines: 2,
		NewStart: 1, NewLines: 2,
		Lines: []string{"-beta", "+BETA", " gamma"},
	}}
	out, err := Apply(content, hunks)
	require.NoError(t, err)
	assert.Equal(t, "x\ny\nz\nalpha\nBETA\ngamma\n", out)
}

func TestApply_PureInsertion(t *testing.T) {
	content := ""
	hunks := []Hunk{{
		OldStart: 0, OldLines: 0,
		NewStart: 1, NewLines: 2,
		Lines: []string{"+one", "+two"},
	}}
	out, err := Apply(content, hunks)
	require.NoError(t, err)
	assert.Equal(t, "one\ntwo", out)
}

func TestApply_DeletionOnly(t *testing.T) {
	content := "a\nb\nc\n"
	hunks := []Hunk{{
		OldStart: 1, OldLines: 3,
		NewStart: 1, NewLines: 2,
		Lines: []string{" a", "-b", " c"},
	}}
	out, err := Apply(content, hunks)
	require.NoError(t, err)
	assert.Equal(t, "a\nc\n", out)
}

func TestApply_PreservesMissingTrailingNewline(t *testing.T) {
	content := "a\nb"
	hunks := []Hunk{{
		OldStart: 1, OldLines: 2,
		NewStart: 1, NewLines: 2,
		Lines: []string{" a", "-b", "+B"},
	}}
	out, err := Apply(content, hunks)
	require.NoError(t, err)
	assert.Equal(t, "a\nB", out)
}

func TestApply_MismatchZeroSideEffect(t *testing.T) {
	content := "a\nb\nc\n"
	hunks := []Hunk{{
		OldStart: 2, OldLines: 1,
		NewStart: 2, NewLines: 1,
		Lines: []string{"-does-not-exist", "+x"},
	}}
	out, err := Apply(content, hunks)
	require.Error(t, err)
	assert.Equal(t, "", out)

	var mismatch *MismatchError
	require.ErrorAs(t, err, &mismatch)
	assert.Equal(t, 0, mismatch.HunkIndex)
	assert.Equal(t, []string{"does-not-exist"}, mismatch.Expected)
}

func TestApply_AmbiguousMatchFails(t *testing.T) {
	// Expected position (line 1) does not match, and the old sequence
	// appears twice elsewhere.
	content := "x\nfoo\ny\nfoo\nz\n"
	hunks := []Hunk{{
		OldStart: 1, OldLines: 1,
		NewStart: 1, NewLines: 1,
		Lines: []string{"-foo", "+bar"},
	}}
	_, err := Apply(content, hunks)
	require.Error(t, err)
	var mismatch *MismatchError
	require.ErrorAs(t, err, &mismatch)
}

func TestApply_ExactPositionPreferredOverAmbiguity(t *testing.T) {
	// "foo" appears twice, but the declared position (line 2) matches,
	// so the hunk applies there.
	content := "foo\nbar\nfoo\n"
	hunks := []Hunk{{
		OldStart: 2, OldLines: 1,
		NewStart: 2, NewLines: 1,
		Lines: []string{"-bar", "+BAR"},
	}}
	out, err := Apply(content, hunks)
	require.NoError(t, err)
	assert.Equal(t, "foo\nBAR\nfoo\n", out)
}

func TestApply_NoHunks(t *testing.T) {
	out, err := Apply("a\nb\n", nil)
	require.NoError(t, err)
	assert.Equal(t, "a\nb\n", out)
}
