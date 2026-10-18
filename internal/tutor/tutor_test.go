package tutor

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"path/filepath"
	"strings"
	"testing"

	"vibetutor/internal/ai"
	"vibetutor/internal/store"
)

type fakeAI struct {
	replies []string
	calls   int
	last    []ai.Message
	err     error
}

func (f *fakeAI) Complete(_ context.Context, msgs []ai.Message) (string, error) {
	f.last = msgs
	if f.err != nil {
		return "", f.err
	}
	reply := f.replies[f.calls%len(f.replies)]
	f.calls++
	return reply, nil
}

func newTutor(t *testing.T, client ai.Client) (*Tutor, store.Store) {
	t.Helper()
	st, err := store.NewSQLite(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return New(client, st, log.New(io.Discard, "", 0)), st
}

func mustSetup(t *testing.T, tut *Tutor, st store.Store, userID int64) {
	t.Helper()
	ctx := context.Background()
	if _, err := tut.Setup(ctx, userID); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	for _, answer := range []string{"русский", "английский", "русский", "A2", "путешествия"} {
		if _, err := tut.Handle(ctx, userID, answer); err != nil {
			t.Fatalf("Handle setup: %v", err)
		}
	}
	sess, _ := st.Load(userID)
	if !sess.Profile.Complete() {
		t.Fatalf("profile incomplete after setup: %+v", sess.Profile)
	}
}

func TestSetupFlow(t *testing.T) {
	tut, st := newTutor(t, &fakeAI{replies: []string{"ok"}})
	ctx := context.Background()

	first, err := tut.Setup(ctx, 1)
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if !strings.Contains(first, setupQuestions[0]) {
		t.Errorf("first question missing: %q", first)
	}

	for i := 1; i < len(setupQuestions); i++ {
		reply, err := tut.Handle(ctx, 1, "answer")
		if err != nil {
			t.Fatalf("Handle: %v", err)
		}
		if !strings.Contains(reply, setupQuestions[i]) {
			t.Errorf("step %d: got %q, want %q", i, reply, setupQuestions[i])
		}
	}

	done, err := tut.Handle(ctx, 1, "свободно говорить")
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !strings.Contains(done, "Профиль сохранён") {
		t.Errorf("setup not finished: %q", done)
	}

	sess, _ := st.Load(1)
	if sess.IsSetup() || sess.Profile.Goal != "свободно говорить" {
		t.Errorf("bad session: %+v", sess)
	}
}

func TestHandleBeforeSetup(t *testing.T) {
	tut, _ := newTutor(t, &fakeAI{replies: []string{"x"}})
	got, err := tut.Handle(context.Background(), 1, "hello")
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if got != MsgNeedSetup {
		t.Errorf("got %q, want %q", got, MsgNeedSetup)
	}
}

func TestPlanAndLesson(t *testing.T) {
	fake := &fakeAI{replies: []string{"ПЛАН УРОКОВ", "ТЕКСТ УРОКА"}}
	tut, st := newTutor(t, fake)
	ctx := context.Background()
	mustSetup(t, tut, st, 1)

	plan, err := tut.Plan(ctx, 1)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if plan != "ПЛАН УРОКОВ" {
		t.Errorf("plan = %q", plan)
	}

	lesson, err := tut.Lesson(ctx, 1, "")
	if err != nil {
		t.Fatalf("Lesson: %v", err)
	}
	if lesson != "ТЕКСТ УРОКА" {
		t.Errorf("lesson = %q", lesson)
	}

	sess, _ := st.Load(1)
	if sess.LessonNumber != 1 || sess.LastLessonOutput != "ТЕКСТ УРОКА" {
		t.Errorf("session after lesson: %+v", sess)
	}

	// Continue the lesson; history should grow and system prompt switch.
	if _, err := tut.Handle(ctx, 1, "my answer"); err != nil {
		t.Fatalf("Handle lesson: %v", err)
	}
	sess, _ = st.Load(1)
	if len(sess.History) != 4 {
		t.Errorf("history length = %d, want 4", len(sess.History))
	}
	if fake.last[0].Role != ai.RoleSystem || !strings.Contains(fake.last[0].Content, "урок номер 1") {
		t.Errorf("lesson system prompt = %q", fake.last[0].Content)
	}
}

func TestLessonRequiresPlan(t *testing.T) {
	tut, st := newTutor(t, &fakeAI{replies: []string{"x"}})
	ctx := context.Background()
	mustSetup(t, tut, st, 1)

	got, err := tut.Lesson(ctx, 1, "")
	if err != nil {
		t.Fatalf("Lesson: %v", err)
	}
	if got != MsgNeedPlan {
		t.Errorf("got %q, want %q", got, MsgNeedPlan)
	}
}

func TestLessonExplicitNumber(t *testing.T) {
	tut, st := newTutor(t, &fakeAI{replies: []string{"plan", "lesson"}})
	ctx := context.Background()
	mustSetup(t, tut, st, 1)
	if _, err := tut.Plan(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := tut.Lesson(ctx, 1, "5"); err != nil {
		t.Fatal(err)
	}
	sess, _ := st.Load(1)
	if sess.LessonNumber != 5 {
		t.Errorf("lesson number = %d, want 5", sess.LessonNumber)
	}
	if _, err := tut.Lesson(ctx, 1, "abc"); err != nil {
		t.Fatal(err)
	}
	if sess, _ := st.Load(1); sess.LessonNumber != 5 {
		t.Errorf("invalid number should not change lesson: %d", sess.LessonNumber)
	}
}

func TestSaveWords(t *testing.T) {
	fake := &fakeAI{replies: []string{
		"plan",
		"lesson text",
		`here you go: [{"term":"hello","translation":"привет"},{"term":"world","translation":"мир"},{"term":"hello","translation":"привет"}]`,
	}}
	tut, st := newTutor(t, fake)
	ctx := context.Background()
	mustSetup(t, tut, st, 1)
	if _, err := tut.Plan(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := tut.Lesson(ctx, 1, ""); err != nil {
		t.Fatal(err)
	}

	msg, err := tut.SaveWords(ctx, 1)
	if err != nil {
		t.Fatalf("SaveWords: %v", err)
	}
	if !strings.Contains(msg, "2 новых") {
		t.Errorf("message = %q", msg)
	}

	sess, _ := st.Load(1)
	if len(sess.Progress.SavedWords) != 2 {
		t.Errorf("saved words = %+v", sess.Progress.SavedWords)
	}
	if len(sess.Progress.CompletedLessons) != 1 || sess.Progress.CompletedLessons[0] != 1 {
		t.Errorf("completed lessons = %v", sess.Progress.CompletedLessons)
	}
	if sess.InLesson() {
		t.Error("should leave lesson mode after saving")
	}
}

func TestSaveWordsNothingToSave(t *testing.T) {
	tut, _ := newTutor(t, &fakeAI{replies: []string{"x"}})
	got, err := tut.SaveWords(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "нечего сохранять") {
		t.Errorf("got %q", got)
	}
}

func TestSaveWordsBadJSON(t *testing.T) {
	fake := &fakeAI{replies: []string{"plan", "lesson", "no json here"}}
	st, err := store.NewSQLite(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	var logs bytes.Buffer
	tut := New(fake, st, log.New(&logs, "", 0))
	ctx := context.Background()
	mustSetup(t, tut, st, 1)
	_, _ = tut.Plan(ctx, 1)
	_, _ = tut.Lesson(ctx, 1, "")

	msg, err := tut.SaveWords(ctx, 1)
	if err != nil {
		t.Fatalf("SaveWords should report the problem to the user, got error: %v", err)
	}
	if !strings.Contains(msg, "разобрать") {
		t.Errorf("message = %q, want a user-facing hint", msg)
	}
	if !strings.Contains(logs.String(), "parse model reply") {
		t.Errorf("parse failure not logged: %q", logs.String())
	}

	// The failed parse must not mark the lesson as saved or completed.
	sess, _ := st.Load(1)
	if len(sess.Progress.SavedWords) != 0 || len(sess.Progress.CompletedLessons) != 0 {
		t.Errorf("progress changed on parse failure: %+v", sess.Progress)
	}
}

func TestAIErrorPropagates(t *testing.T) {
	fake := &fakeAI{replies: []string{"x"}, err: errors.New("boom")}
	tut, st := newTutor(t, fake)
	ctx := context.Background()
	mustSetup(t, tut, st, 1)
	if _, err := tut.Plan(ctx, 1); err == nil {
		t.Fatal("expected error")
	}
}

func TestReset(t *testing.T) {
	tut, st := newTutor(t, &fakeAI{replies: []string{"x"}})
	ctx := context.Background()
	mustSetup(t, tut, st, 1)
	if _, err := tut.Reset(ctx, 1); err != nil {
		t.Fatal(err)
	}
	sess, _ := st.Load(1)
	if sess.Profile.Complete() {
		t.Error("profile should be cleared")
	}
}

func TestProfile(t *testing.T) {
	fake := &fakeAI{replies: []string{"x"}}
	tut, st := newTutor(t, fake)
	ctx := context.Background()

	if got, _ := tut.Profile(ctx, 1); got != MsgNeedSetup {
		t.Errorf("got %q", got)
	}
	mustSetup(t, tut, st, 1)
	got, err := tut.Profile(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "английский") {
		t.Errorf("profile = %q", got)
	}
}

func TestParseWords(t *testing.T) {
	words, err := parseWords(`prefix [{"term":"a","translation":"б"}] suffix`)
	if err != nil {
		t.Fatalf("parseWords: %v", err)
	}
	if len(words) != 1 || words[0].Term != "a" {
		t.Errorf("words = %+v", words)
	}
	if _, err := parseWords("nothing"); err == nil {
		t.Error("expected error")
	}
}

func TestParseWordsTrailingBrackets(t *testing.T) {
	words, err := parseWords(`[{"term":"a","translation":"б"}] (см. [1])`)
	if err != nil {
		t.Fatalf("parseWords: %v", err)
	}
	if len(words) != 1 || words[0].Term != "a" {
		t.Errorf("words = %+v", words)
	}
}

func TestSplitHistoryLimit(t *testing.T) {
	history := make([]ai.Message, maxHistory+10)
	for i := range history {
		history[i] = ai.Message{Role: ai.RoleUser, Content: "m"}
	}
	trimmed := trimHistory(history)
	if len(trimmed) != maxHistory {
		t.Errorf("len = %d, want %d", len(trimmed), maxHistory)
	}
}
