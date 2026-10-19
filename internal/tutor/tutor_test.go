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
	"unicode/utf8"

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
	if sess.LastLessonOutput != "" || sess.LessonNumber != 0 {
		t.Errorf("lesson state not cleared after save: %+v", sess)
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

func TestSaveWordsWrongKeysKeepsLesson(t *testing.T) {
	fake := &fakeAI{replies: []string{
		"plan", "lesson text", `[{"word":"hello","meaning":"привет"}]`,
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
	if !strings.Contains(msg, "разобрать") {
		t.Errorf("message = %q, want a parse hint", msg)
	}

	sess, _ := st.Load(1)
	if len(sess.Progress.SavedWords) != 0 || len(sess.Progress.CompletedLessons) != 0 {
		t.Errorf("progress changed on unusable array: %+v", sess.Progress)
	}
	if sess.LastLessonOutput == "" || sess.LessonNumber != 1 {
		t.Errorf("lesson state cleared on unusable array: %+v", sess)
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

func TestEmptyReplyNotPersisted(t *testing.T) {
	fake := &fakeAI{replies: []string{"plan", "lesson", "   "}}
	tut, st := newTutor(t, fake)
	ctx := context.Background()
	mustSetup(t, tut, st, 1)
	if _, err := tut.Plan(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := tut.Lesson(ctx, 1, ""); err != nil {
		t.Fatal(err)
	}

	before, _ := st.Load(1)
	reply, err := tut.Handle(ctx, 1, "my answer")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(reply) != "" {
		t.Errorf("reply = %q, want empty", reply)
	}
	after, _ := st.Load(1)
	if len(after.History) != len(before.History) {
		t.Errorf("history changed on empty reply: %d -> %d", len(before.History), len(after.History))
	}
	if after.LastLessonOutput != before.LastLessonOutput {
		t.Errorf("lesson output changed on empty reply: %q -> %q", before.LastLessonOutput, after.LastLessonOutput)
	}
}

func TestPlanEmptyReplyKeepsPreviousPlan(t *testing.T) {
	fake := &fakeAI{replies: []string{"ПЛАН", "   "}}
	tut, st := newTutor(t, fake)
	ctx := context.Background()
	mustSetup(t, tut, st, 1)
	if _, err := tut.Plan(ctx, 1); err != nil {
		t.Fatal(err)
	}
	before, _ := st.Load(1)

	reply, err := tut.Plan(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(reply) != "" {
		t.Errorf("reply = %q, want empty", reply)
	}
	after, _ := st.Load(1)
	if after.Plan != "ПЛАН" {
		t.Errorf("plan = %q, want previous plan kept", after.Plan)
	}
	if after.Mode != before.Mode || len(after.History) != len(before.History) {
		t.Errorf("session changed on empty plan: %+v", after)
	}
}

func TestLessonEmptyReplyKeepsPreviousLesson(t *testing.T) {
	fake := &fakeAI{replies: []string{"plan", "УРОК", "   "}}
	tut, st := newTutor(t, fake)
	ctx := context.Background()
	mustSetup(t, tut, st, 1)
	if _, err := tut.Plan(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := tut.Lesson(ctx, 1, ""); err != nil {
		t.Fatal(err)
	}
	before, _ := st.Load(1)

	reply, err := tut.Lesson(ctx, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(reply) != "" {
		t.Errorf("reply = %q, want empty", reply)
	}
	after, _ := st.Load(1)
	if after.LastLessonOutput != "УРОК" || after.LessonNumber != before.LessonNumber {
		t.Errorf("lesson state changed on empty reply: %+v", after)
	}
	if len(after.History) != len(before.History) {
		t.Errorf("history changed on empty reply: %d -> %d", len(before.History), len(after.History))
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

func TestSetupClearsLessonAndReviewState(t *testing.T) {
	fake := &fakeAI{replies: []string{"plan", "lesson"}}
	tut, st := newTutor(t, fake)
	ctx := context.Background()
	mustSetup(t, tut, st, 1)
	if _, err := tut.Plan(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := tut.Lesson(ctx, 1, ""); err != nil {
		t.Fatal(err)
	}
	seedWords(t, st, 1, store.Word{Term: "hello", Translation: "привет"})
	if _, err := tut.Review(1); err != nil {
		t.Fatal(err)
	}
	before, _ := st.Load(1)

	if _, err := tut.Setup(ctx, 1); err != nil {
		t.Fatal(err)
	}
	after, _ := st.Load(1)
	if after.LastLessonOutput != "" || after.LessonNumber != 0 {
		t.Errorf("lesson state not cleared: %+v", after)
	}
	if after.Plan != "" || len(after.History) != 0 {
		t.Errorf("chat context not cleared: plan=%q history=%d", after.Plan, len(after.History))
	}
	if after.ReviewTerm != "" || after.ReviewGen == before.ReviewGen {
		t.Errorf("review state not disarmed: term=%q gen=%d (was %d)", after.ReviewTerm, after.ReviewGen, before.ReviewGen)
	}
}

func TestResetPreservesReviewGen(t *testing.T) {
	tut, st := newTutor(t, &fakeAI{replies: []string{"x"}})
	seedWords(t, st, 1, store.Word{Term: "hello", Translation: "привет"})
	if _, err := tut.Review(1); err != nil {
		t.Fatal(err)
	}
	before, _ := st.Load(1)
	if before.ReviewGen == 0 {
		t.Fatal("expected a nonzero generation after starting a review")
	}
	if _, err := tut.Reset(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	after, _ := st.Load(1)
	if after.ReviewGen != before.ReviewGen {
		t.Errorf("generation = %d, want %d preserved across reset", after.ReviewGen, before.ReviewGen)
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

func TestParseWordsEmptyArray(t *testing.T) {
	words, err := parseWords("[]")
	if err != nil {
		t.Fatalf("parseWords: %v", err)
	}
	if len(words) != 0 {
		t.Errorf("words = %+v, want empty", words)
	}
}

func TestParseWordsRequiresTerm(t *testing.T) {
	if _, err := parseWords(`[{"word":"a","meaning":"б"}]`); err == nil {
		t.Error("expected error for an array without terms")
	}
}

func TestParseWordsRequiresTranslation(t *testing.T) {
	if _, err := parseWords(`[{"term":"a"}]`); err == nil {
		t.Error("expected error for an array without translations")
	}
}

func TestAddWordsSkipsEmptyTranslation(t *testing.T) {
	var p store.Progress
	added := addWords(&p, []store.Word{
		{Term: "a", Translation: " "},
		{Term: "b", Translation: "б"},
		{Term: "c"},
	})
	if added != 1 || len(p.SavedWords) != 1 || p.SavedWords[0].Term != "b" {
		t.Errorf("added = %d, words = %+v", added, p.SavedWords)
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

func TestParseWordsLeadingBrackets(t *testing.T) {
	words, err := parseWords(`Смотри пример [1] и словарь: [{"term":"a","translation":"б"}]`)
	if err != nil {
		t.Fatalf("parseWords: %v", err)
	}
	if len(words) != 1 || words[0].Term != "a" {
		t.Errorf("words = %+v", words)
	}
}

func TestLessonAccumulatesOutput(t *testing.T) {
	fake := &fakeAI{replies: []string{
		"plan", "УРОК ЧАСТЬ 1", "УРОК ЧАСТЬ 2",
		`[{"term":"a","translation":"б"}]`,
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
	if _, err := tut.Handle(ctx, 1, "мой ответ"); err != nil {
		t.Fatal(err)
	}

	sess, _ := st.Load(1)
	if !strings.Contains(sess.LastLessonOutput, "УРОК ЧАСТЬ 1") ||
		!strings.Contains(sess.LastLessonOutput, "УРОК ЧАСТЬ 2") {
		t.Errorf("lesson output not accumulated: %q", sess.LastLessonOutput)
	}

	if _, err := tut.SaveWords(ctx, 1); err != nil {
		t.Fatal(err)
	}
	extractInput := fake.last[1].Content
	if !strings.Contains(extractInput, "УРОК ЧАСТЬ 1") || !strings.Contains(extractInput, "УРОК ЧАСТЬ 2") {
		t.Errorf("save received only part of the lesson: %q", extractInput)
	}
}

func TestPlanResetsLessonState(t *testing.T) {
	fake := &fakeAI{replies: []string{"plan", "lesson", "plan2"}}
	tut, st := newTutor(t, fake)
	ctx := context.Background()
	mustSetup(t, tut, st, 1)
	if _, err := tut.Plan(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := tut.Lesson(ctx, 1, ""); err != nil {
		t.Fatal(err)
	}
	if sess, _ := st.Load(1); sess.LessonNumber != 1 || sess.LastLessonOutput == "" {
		t.Fatalf("lesson state not set: %+v", sess)
	}

	if _, err := tut.Plan(ctx, 1); err != nil {
		t.Fatal(err)
	}
	sess, _ := st.Load(1)
	if sess.LessonNumber != 0 || sess.LastLessonOutput != "" {
		t.Errorf("plan did not clear stale lesson state: %+v", sess)
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

func TestAppendLessonOutputBounded(t *testing.T) {
	out := appendLessonOutput("", strings.Repeat("a", maxLessonOutput+500))
	if len(out) > maxLessonOutput {
		t.Errorf("len = %d, want <= %d", len(out), maxLessonOutput)
	}

	out = ""
	for i := 0; i < 10; i++ {
		out = appendLessonOutput(out, strings.Repeat("привет", 1000))
	}
	if len(out) > maxLessonOutput {
		t.Errorf("len = %d, want <= %d", len(out), maxLessonOutput)
	}
	if !utf8.ValidString(out) {
		t.Error("truncated output is not valid UTF-8")
	}
	if !strings.HasSuffix(out, "привет") {
		t.Error("truncated output lost the most recent reply")
	}
}
