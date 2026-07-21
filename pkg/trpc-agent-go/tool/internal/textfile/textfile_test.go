//
// Tencent is pleased to support the open source community by making
// trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//
//

package textfile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectLineEnding(t *testing.T) {
	assert.Equal(t, "\r\n", DetectLineEnding([]byte("alpha\r\nbeta")))
	assert.Equal(t, "\n", DetectLineEnding([]byte("alpha\nbeta")))
	assert.Equal(t, "\n", DetectLineEnding(nil))
}

func TestApplyLineEnding(t *testing.T) {
	assert.Equal(t, "a\r\nb\r\n", ApplyLineEnding("a\nb\n", "\r\n"))
	assert.Equal(t, "a\nb\n", ApplyLineEnding("a\nb\n", "\n"))
}

func TestNormalizeNewlines(t *testing.T) {
	assert.Equal(t,
		"alpha\nbeta\ngamma\n",
		NormalizeNewlines("alpha\r\nbeta\rgamma\n"),
	)
}

func TestCountLines(t *testing.T) {
	assert.Zero(t, CountLines(""))
	assert.Equal(t, 2, CountLines("alpha\nbeta"))
	assert.Equal(t, 2, CountLines("alpha\nbeta\n"))
}

func TestSplitTextLines(t *testing.T) {
	assert.Empty(t, SplitTextLines(""))
	assert.Equal(t,
		[]string{"alpha", "beta"},
		SplitTextLines("alpha\nbeta\n"),
	)
	assert.Equal(t,
		[]string{"alpha", "beta"},
		SplitTextLines("alpha\nbeta"),
	)
}

func TestDecodeEncodeTextBytes_UTF8(t *testing.T) {
	content, encoding, err := DecodeTextBytes([]byte("one\r\ntwo\r\n"))
	require.NoError(t, err)
	assert.Equal(t, "one\ntwo\n", content)
	assert.Equal(t, "utf8", encoding)

	encoded, err := EncodeTextBytes("alpha\nbeta", "utf8", "\n")
	require.NoError(t, err)
	assert.Equal(t, []byte("alpha\nbeta"), encoded)

	encodedCRLF, err := EncodeTextBytes("alpha\nbeta\n", "utf8", "\r\n")
	require.NoError(t, err)
	assert.Equal(t, []byte("alpha\r\nbeta\r\n"), encodedCRLF)
}

func TestDecodeEncodeTextBytes_UTF16LE(t *testing.T) {
	encoded, err := EncodeTextBytes("alpha\nbeta\n", "utf16le", "\r\n")
	require.NoError(t, err)

	decoded, encoding, err := DecodeTextBytes(encoded)
	require.NoError(t, err)
	assert.Equal(t, "utf16le", encoding)
	assert.Equal(t, "alpha\nbeta\n", decoded)
}

func TestIsProbablyBinary(t *testing.T) {
	assert.True(t, IsProbablyBinary([]byte("a\x00b")))
	assert.False(t, IsProbablyBinary([]byte{0xff, 0xfe, 'a', 0x00}))
	assert.False(t, IsProbablyBinary([]byte("plain text")))
}

func TestNormalizeQuotes(t *testing.T) {
	assert.Equal(t,
		"\"quote\" and 'apostrophe'",
		NormalizeQuotes("“quote” and ‘apostrophe’"),
	)
}

func TestFindActualString(t *testing.T) {
	// Direct hit.
	assert.Equal(t, "plain", FindActualString("a plain b", "plain"))
	// Fuzzy single quotes + apostrophe.
	actual := FindActualString("‘quoted text’ and don’t", "'quoted text' and don't")
	assert.Equal(t, "‘quoted text’ and don’t", actual)
	// Fuzzy double quotes.
	actualDouble := FindActualString("“quoted text”", "\"quoted text\"")
	assert.Equal(t, "“quoted text”", actualDouble)
	// Miss.
	assert.Equal(t, "", FindActualString("alpha", "beta"))
}

func TestPreserveQuoteStyle(t *testing.T) {
	// Identical strings: new text passes through unchanged.
	assert.Equal(t,
		"'new text'",
		PreserveQuoteStyle("'old'", "'old'", "'new text'"),
	)
	// Fuzzy single quotes preserved in replacement.
	actual := FindActualString("‘quoted text’ and don’t", "'quoted text' and don't")
	require.Equal(t, "‘quoted text’ and don’t", actual)
	assert.Equal(t,
		"‘new text’ and can’t",
		PreserveQuoteStyle(
			"'quoted text' and don't",
			actual,
			"'new text' and can't",
		),
	)
	// Fuzzy double quotes preserved in replacement.
	actualDouble := FindActualString("“quoted text”", "\"quoted text\"")
	require.Equal(t, "“quoted text”", actualDouble)
	assert.Equal(t,
		"“new text”",
		PreserveQuoteStyle("\"quoted text\"", actualDouble, "\"new text\""),
	)
}
