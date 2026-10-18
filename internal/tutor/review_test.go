package tutor

import (
	"strings"
	"testing"
	"time"

	"vibetutor/internal/store"
)

func seedWords(t *testing.T, st store.Store, userID int64, words ...store.Word) {
	t.Helper()
	sess, err := st.Load(userID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	sess.Progress.SavedWords = append(sess.Progress.SavedWords, words...)
	if err := st.Save(userID, sess); err != nil {
		t.Fatalf("save: %v", err)
	}
}

func TestReviewEmptyDictionary(t *testing.T) {
	tut, _ := newTutor(t, &fakeAI{replies: []string{"x"}})
	step, err := tut.Review(1)
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	if step.Actions != nil {
		t.Errorf("expected no actions, got %v", step.Actions)
	}
}

func TestReviewFlow(t *testing.T) {
	tut, st := newTutor(t, &fakeAI{replies: []string{"x"}})
	seedWords(t, st, 1, store.Word{Term: "hello", Translation: "привет"})

	step, err := tut.Review(1)
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	if len(step.Actions) != 1 || step.Actions[0].Data != reviewShow {
		t.Fatalf("expected show action, got %v", step.Actions)
	}

	revealed, err := tut.ReviewAction(1, reviewShow)
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	if len(revealed.Actions) != 4 {
		t.Fatalf("expected 4 rating actions, got %v", revealed.Actions)
	}
	if revealed.Text == "" || !strings.Contains(revealed.Text, "привет") {
		t.Errorf("translation missing: %q", revealed.Text)
	}

	done, err := tut.ReviewAction(1, reviewGood)
	if err != nil {
		t.Fatalf("rate: %v", err)
	}
	if done.Actions != nil {
		t.Errorf("expected session to finish, got %v", done.Actions)
	}

	sess, _ := st.Load(1)
	if sess.ReviewTerm != "" {
		t.Errorf("ReviewTerm not cleared: %q", sess.ReviewTerm)
	}
	w := sess.Progress.SavedWords[0]
	if w.Reps != 1 || w.Interval != 1 || !w.DueAt.After(time.Now()) {
		t.Errorf("word not rescheduled: %+v", w)
	}
}

func TestReviewDueFiltering(t *testing.T) {
	tut, st := newTutor(t, &fakeAI{replies: []string{"x"}})
	seedWords(t, st, 1,
		store.Word{Term: "later", Translation: "позже", DueAt: time.Now().Add(time.Hour)},
		store.Word{Term: "now", Translation: "сейчас"},
	)

	step, err := tut.Review(1)
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	if !strings.Contains(step.Text, "now") || strings.Contains(step.Text, "later") {
		t.Errorf("wrong word selected: %q", step.Text)
	}

	// Rating the only due word should finish the session.
	done, err := tut.ReviewAction(1, reviewEasy)
	if err != nil {
		t.Fatalf("rate: %v", err)
	}
	if done.Actions != nil {
		t.Errorf("expected completion, got %v", done.Actions)
	}
}

func TestReviewActionWithoutSession(t *testing.T) {
	tut, st := newTutor(t, &fakeAI{replies: []string{"x"}})
	seedWords(t, st, 1, store.Word{Term: "hello", Translation: "привет"})

	if _, err := tut.ReviewAction(1, reviewShow); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReviewUnknownAction(t *testing.T) {
	tut, st := newTutor(t, &fakeAI{replies: []string{"x"}})
	seedWords(t, st, 1, store.Word{Term: "hello", Translation: "привет"})
	if _, err := tut.Review(1); err != nil {
		t.Fatal(err)
	}
	if _, err := tut.ReviewAction(1, "bogus"); err == nil {
		t.Fatal("expected error")
	}
}

func TestSchedule(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	t.Run("again resets", func(t *testing.T) {
		w := store.Word{Ease: 2.5, Reps: 3, Interval: 10}
		schedule(&w, 1, now)
		if w.Reps != 0 || w.Interval != 1 {
			t.Errorf("got reps=%d interval=%d", w.Reps, w.Interval)
		}
		if !w.DueAt.Equal(now.AddDate(0, 0, 1)) {
			t.Errorf("due = %v", w.DueAt)
		}
		if w.Ease >= 2.5 {
			t.Errorf("ease should drop: %v", w.Ease)
		}
	})

	t.Run("good grows interval", func(t *testing.T) {
		w := store.Word{}
		schedule(&w, 4, now)
		if w.Reps != 1 || w.Interval != 1 {
			t.Errorf("first review: reps=%d interval=%d", w.Reps, w.Interval)
		}
		schedule(&w, 4, now)
		if w.Reps != 2 || w.Interval != 6 {
			t.Errorf("second review: reps=%d interval=%d", w.Reps, w.Interval)
		}
		schedule(&w, 4, now)
		if w.Reps != 3 || w.Interval < 6 {
			t.Errorf("third review: reps=%d interval=%d", w.Reps, w.Interval)
		}
	})

	t.Run("ease clamped", func(t *testing.T) {
		w := store.Word{Ease: 1.3}
		for i := 0; i < 10; i++ {
			schedule(&w, 1, now)
		}
		if w.Ease < 1.3 {
			t.Errorf("ease below floor: %v", w.Ease)
		}
	})
}
