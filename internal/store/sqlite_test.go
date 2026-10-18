package store

import (
	"path/filepath"
	"testing"

	"vibetutor/internal/ai"
)

func newStore(t *testing.T) *SQLiteStore {
	t.Helper()
	s, err := NewSQLite(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestSQLiteStoreRoundTrip(t *testing.T) {
	s := newStore(t)

	sess, err := s.Load(7)
	if err != nil {
		t.Fatalf("Load new: %v", err)
	}
	if sess.Mode != ModeChat {
		t.Errorf("new session mode = %q", sess.Mode)
	}

	sess.Profile = Profile{Native: "ru", Learning: "en", Explanation: "ru", Level: "A2", Goal: "trip"}
	sess.Progress.SavedWords = []Word{{Term: "hello", Translation: "привет"}}
	sess.Progress.CompletedLessons = []int{1, 2}
	sess.History = []ai.Message{{Role: ai.RoleUser, Content: "hi"}, {Role: ai.RoleAssistant, Content: "hey"}}
	if err := s.Save(7, sess); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := s.Load(7)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !got.Profile.Complete() || got.Profile.Learning != "en" {
		t.Errorf("profile not persisted: %+v", got.Profile)
	}
	if len(got.Progress.SavedWords) != 1 || got.Progress.SavedWords[0].Term != "hello" {
		t.Errorf("words not persisted: %+v", got.Progress.SavedWords)
	}
	if len(got.Progress.CompletedLessons) != 2 {
		t.Errorf("lessons not persisted: %+v", got.Progress.CompletedLessons)
	}
	if len(got.History) != 2 || got.History[1].Content != "hey" {
		t.Errorf("history not persisted: %+v", got.History)
	}
}

func TestSQLiteStoreUpsert(t *testing.T) {
	s := newStore(t)

	if err := s.Save(1, &Session{Plan: "first"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(1, &Session{Plan: "second"}); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Load(1)
	if got.Plan != "second" {
		t.Errorf("plan = %q, want second", got.Plan)
	}
}

func TestSQLiteStoreSeparateUsers(t *testing.T) {
	s := newStore(t)
	if err := s.Save(1, &Session{Plan: "one"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(2, &Session{Plan: "two"}); err != nil {
		t.Fatal(err)
	}
	a, _ := s.Load(1)
	b, _ := s.Load(2)
	if a.Plan != "one" || b.Plan != "two" {
		t.Errorf("users not isolated: %q %q", a.Plan, b.Plan)
	}
}

func TestNextLesson(t *testing.T) {
	s := &Session{Progress: Progress{CompletedLessons: []int{1, 3, 2}}}
	if got := s.NextLesson(); got != 4 {
		t.Errorf("NextLesson = %d, want 4", got)
	}
	if (&Session{}).NextLesson() != 1 {
		t.Error("empty progress should start at 1")
	}
}
