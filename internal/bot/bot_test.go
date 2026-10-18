package bot

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"vibetutor/internal/tutor"
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

func TestReviewMarkup(t *testing.T) {
	step := &tutor.ReviewStep{Actions: []tutor.ReviewAction{{Data: "show", Label: "Показать"}}}
	markup := reviewMarkup(step)
	if markup == nil || len(markup.InlineKeyboard) != 1 || len(markup.InlineKeyboard[0]) != 1 {
		t.Fatalf("markup = %+v", markup)
	}
	btn := markup.InlineKeyboard[0][0]
	if btn.Unique != reviewButtonUnique || btn.Data != "show" || btn.Text != "Показать" {
		t.Errorf("button = %+v", btn)
	}

	if reviewMarkup(&tutor.ReviewStep{}) != nil {
		t.Error("expected nil markup when there are no actions")
	}
	if reviewMarkup(nil) != nil {
		t.Error("expected nil markup for nil step")
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

func TestSplitMessageInvalidUTF8Progress(t *testing.T) {
	text := strings.Repeat("\x80", 20)
	done := make(chan []string, 1)
	go func() { done <- SplitMessage(text, 10) }()
	select {
	case chunks := <-done:
		if len(chunks) != 2 {
			t.Fatalf("got %d chunks, want 2", len(chunks))
		}
		for _, c := range chunks {
			if len(c) > 10 {
				t.Errorf("chunk too long: %d", len(c))
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SplitMessage did not terminate on invalid UTF-8")
	}
}

func TestSplitMessageNoEmptyChunks(t *testing.T) {
	text := "ab" + strings.Repeat(" ", 30) + "cd"
	chunks := SplitMessage(text, 10)
	for _, c := range chunks {
		if c == "" {
			t.Fatalf("empty chunk in %q", chunks)
		}
	}
	if len(chunks) != 2 || chunks[0] != "ab" || chunks[1] != "cd" {
		t.Errorf("chunks = %q, want [ab cd]", chunks)
	}
}

func TestOutgoingChunks(t *testing.T) {
	if chunks := outgoingChunks("hi"); len(chunks) != 1 || chunks[0] != "hi" {
		t.Errorf("chunks = %v", chunks)
	}
	if chunks := outgoingChunks("   \n "); len(chunks) != 1 || chunks[0] != emptyReplyMsg {
		t.Errorf("empty reply chunks = %v, want fallback", chunks)
	}
}

func TestCommandPattern(t *testing.T) {
	commands := []string{"/help", "/lesson 3", "/start@vibetutor_bot", "/save", "  /reset"}
	for _, text := range commands {
		if !commandPattern.MatchString(strings.TrimSpace(text)) {
			t.Errorf("%q should be treated as a command", text)
		}
	}

	answers := []string{
		"/b/ sound",
		"и/или",
		"/path/to/file",
		"hello /world",
		"2/3",
	}
	for _, text := range answers {
		if commandPattern.MatchString(strings.TrimSpace(text)) {
			t.Errorf("%q should not be treated as a command", text)
		}
	}
}
