package store

import "testing"

func TestFileStoreRoundTrip(t *testing.T) {
	s, err := NewFile(t.TempDir())
	if err != nil {
		t.Fatalf("NewFile: %v", err)
	}

	sess, err := s.Load(7)
	if err != nil {
		t.Fatalf("Load new: %v", err)
	}
	if sess.Mode != ModeChat {
		t.Errorf("new session mode = %q", sess.Mode)
	}

	sess.Profile = Profile{Native: "ru", Learning: "en", Explanation: "ru", Level: "A2", Goal: "trip"}
	sess.Progress.SavedWords = []Word{{Term: "hello", Translation: "привет"}}
	if err := s.Save(7, sess); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := s.Load(7)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !got.Profile.Complete() {
		t.Errorf("profile not persisted: %+v", got.Profile)
	}
	if len(got.Progress.SavedWords) != 1 || got.Progress.SavedWords[0].Term != "hello" {
		t.Errorf("words not persisted: %+v", got.Progress.SavedWords)
	}
}

func TestFileStoreSeparateUsers(t *testing.T) {
	s, _ := NewFile(t.TempDir())
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
