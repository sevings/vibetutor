package bot

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSplitMessageShort(t *testing.T) {
	chunks := SplitMessage("hello", 100)
	if len(chunks) != 1 || chunks[0] != "hello" {
		t.Fatalf("chunks = %v", chunks)
	}
}

func TestSplitMessageEmpty(t *testing.T) {
	if chunks := SplitMessage("   \n  ", 100); chunks != nil {
		t.Errorf("expected nil, got %v", chunks)
	}
}

func TestSplitMessageLong(t *testing.T) {
	text := strings.Repeat("a", 250)
	chunks := SplitMessage(text, 100)
	if len(chunks) != 3 {
		t.Fatalf("got %d chunks", len(chunks))
	}
	for _, c := range chunks {
		if len(c) > 100 {
			t.Errorf("chunk too long: %d", len(c))
		}
	}
	if strings.Join(chunks, "") != text {
		t.Error("chunks do not reconstruct the text")
	}
}

func TestSplitMessageNewlineBoundary(t *testing.T) {
	text := strings.Repeat("line\n", 40)
	chunks := SplitMessage(text, 25)
	for _, c := range chunks {
		if len(c) > 25 {
			t.Errorf("chunk too long: %d", len(c))
		}
		if strings.HasPrefix(c, "\n") || strings.HasSuffix(c, "\n") {
			t.Errorf("chunk has stray newline: %q", c)
		}
	}
}

func TestSplitMessageUTF8(t *testing.T) {
	text := strings.Repeat("привет", 40) // 6 bytes per word
	chunks := SplitMessage(text, 7)
	for _, c := range chunks {
		if !utf8.ValidString(c) {
			t.Errorf("invalid utf8 chunk: %q", c)
		}
	}
	if strings.Join(chunks, "") != text {
		t.Error("chunks do not reconstruct the text")
	}
}
