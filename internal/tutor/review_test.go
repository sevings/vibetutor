package tutor

import (
	"strings"
	"sync"
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
	if len(step.Actions) != 1 {
		t.Fatalf("expected show action, got %v", step.Actions)
	}

	revealed, err := tut.ReviewAction(1, step.Actions[0].Data)
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	if len(revealed.Actions) != 4 {
		t.Fatalf("expected 4 rating actions, got %v", revealed.Actions)
	}
	if revealed.Text == "" || !strings.Contains(revealed.Text, "привет") {
		t.Errorf("translation missing: %q", revealed.Text)
	}

	done, err := tut.ReviewAction(1, revealed.Actions[2].Data)
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

	revealed, err := tut.ReviewAction(1, step.Actions[0].Data)
	if err != nil {
		t.Fatalf("show: %v", err)
	}

	// Rating the only due word should finish the session.
	done, err := tut.ReviewAction(1, revealed.Actions[3].Data)
	if err != nil {
		t.Fatalf("rate: %v", err)
	}
	if done.Actions != nil {
		t.Errorf("expected completion, got %v", done.Actions)
	}
}

func TestReviewActionWithoutArmedWord(t *testing.T) {
	tut, st := newTutor(t, &fakeAI{replies: []string{"x"}})
	seedWords(t, st, 1, store.Word{Term: "hello", Translation: "привет"})

	step, err := tut.ReviewAction(1, encodeAction(reviewShow, 0, 0))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !step.Stale {
		t.Errorf("expected stale step, got %+v", step)
	}
	if step.Actions != nil {
		t.Errorf("expected stale step without actions, got %v", step.Actions)
	}
}

func TestReviewStaleButton(t *testing.T) {
	tut, st := newTutor(t, &fakeAI{replies: []string{"x"}})
	seedWords(t, st, 1,
		store.Word{Term: "one", Translation: "один"},
		store.Word{Term: "two", Translation: "два"},
	)

	step, err := tut.Review(1)
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	stale := step.Actions[0].Data

	revealed, err := tut.ReviewAction(1, stale)
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	if _, err := tut.ReviewAction(1, revealed.Actions[2].Data); err != nil {
		t.Fatalf("rate: %v", err)
	}

	// The first message's button now points at a word that is no longer armed.
	got, err := tut.ReviewAction(1, stale)
	if err != nil {
		t.Fatalf("stale show: %v", err)
	}
	if !got.Stale {
		t.Errorf("expected stale step, got %+v", got)
	}
	if got.Actions != nil {
		t.Errorf("expected stale step, got %v", got.Actions)
	}
}

func TestReviewStaleButtonAcrossSessions(t *testing.T) {
	tut, st := newTutor(t, &fakeAI{replies: []string{"x"}})
	seedWords(t, st, 1, store.Word{Term: "hello", Translation: "привет"})

	first, err := tut.Review(1)
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	oldData := first.Actions[0].Data

	// Start a new session; the same word is armed again but belongs to a new
	// generation, so the previous message's button must be rejected.
	if _, err := tut.Review(1); err != nil {
		t.Fatalf("Review again: %v", err)
	}

	got, err := tut.ReviewAction(1, oldData)
	if err != nil {
		t.Fatalf("stale show: %v", err)
	}
	if !got.Stale {
		t.Errorf("expected stale step, got %+v", got)
	}
}

func TestCountDue(t *testing.T) {
	now := time.Now()
	sess := &store.Session{Progress: store.Progress{SavedWords: []store.Word{
		{Term: "a"},
		{Term: "b", DueAt: now.Add(time.Hour)},
		{Term: "c", DueAt: now},
	}}}
	if got := countDue(sess, now); got != 2 {
		t.Errorf("countDue = %d, want 2", got)
	}
}

func TestReviewConcurrentRating(t *testing.T) {
	tut, st := newTutor(t, &fakeAI{replies: []string{"x"}})
	seedWords(t, st, 1, store.Word{Term: "hello", Translation: "привет"})

	step, err := tut.Review(1)
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	revealed, err := tut.ReviewAction(1, step.Actions[0].Data)
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	rating := revealed.Actions[2].Data // "Помню"

	// Rapid repeated taps must not schedule the same word more than once.
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := tut.ReviewAction(1, rating); err != nil {
				t.Errorf("review action: %v", err)
			}
		}()
	}
	wg.Wait()

	sess, _ := st.Load(1)
	if w := sess.Progress.SavedWords[0]; w.Reps != 1 {
		t.Errorf("word rated %d times, want 1: %+v", w.Reps, w)
	}
	if sess.ReviewTerm != "" {
		t.Errorf("ReviewTerm = %q, want empty after completion", sess.ReviewTerm)
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
