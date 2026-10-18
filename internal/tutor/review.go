package tutor

import (
	"fmt"
	"math"
	"strings"
	"time"

	"vibetutor/internal/store"
)

// Review action codes exchanged with the transport layer.
const (
	reviewShow  = "show"
	reviewAgain = "again"
	reviewHard  = "hard"
	reviewGood  = "good"
	reviewEasy  = "easy"
)

const defaultEase = 2.5

// ReviewAction is a transport-neutral button: Data is the code sent back to
// ReviewAction, Label is what the user sees.
type ReviewAction struct {
	Data  string
	Label string
}

// ReviewStep is one screen of a review session.
type ReviewStep struct {
	Text    string
	Actions []ReviewAction
}

// Review starts (or resumes) a review session and returns the first screen.
func (t *Tutor) Review(userID int64) (*ReviewStep, error) {
	sess, err := t.store.Load(userID)
	if err != nil {
		return nil, err
	}
	if len(sess.Progress.SavedWords) == 0 {
		return &ReviewStep{Text: "Словарь пуст. Пройди урок и сохрани слова командой /save."}, nil
	}

	step, err := t.nextReviewStep(sess)
	if err != nil {
		return nil, err
	}
	if err := t.store.Save(userID, sess); err != nil {
		return nil, err
	}
	return step, nil
}

// ReviewAction handles a button press during a review session.
func (t *Tutor) ReviewAction(userID int64, data string) (*ReviewStep, error) {
	sess, err := t.store.Load(userID)
	if err != nil {
		return nil, err
	}

	idx := wordIndex(sess, sess.ReviewTerm)
	if idx < 0 {
		return &ReviewStep{Text: "Сессия повторения не найдена. Запусти её заново командой /review."}, nil
	}

	if data == reviewShow {
		return revealStep(sess.Progress.SavedWords[idx]), nil
	}

	quality, ok := qualityOf(data)
	if !ok {
		return nil, fmt.Errorf("неизвестное действие повторения %q", data)
	}

	schedule(&sess.Progress.SavedWords[idx], quality, time.Now())
	sess.ReviewTerm = ""

	step, err := t.nextReviewStep(sess)
	if err != nil {
		return nil, err
	}
	if err := t.store.Save(userID, sess); err != nil {
		return nil, err
	}
	return step, nil
}

// nextReviewStep selects the next due word and arms the session for it.
func (t *Tutor) nextReviewStep(sess *store.Session) (*ReviewStep, error) {
	now := time.Now()
	idx := dueIndex(sess, now)
	if idx < 0 {
		sess.ReviewTerm = ""
		return &ReviewStep{Text: "Повторение завершено! Слов к повторению больше нет."}, nil
	}

	sess.ReviewTerm = sess.Progress.SavedWords[idx].Term
	pending := countDue(sess, now)
	return &ReviewStep{
		Text: fmt.Sprintf("Слово: %s\n\nВспомни перевод и нажми «Показать». К повторению: %d.", sess.ReviewTerm, pending),
		Actions: []ReviewAction{
			{Data: reviewShow, Label: "Показать"},
		},
	}, nil
}

// revealStep shows the translation and the self-assessment buttons.
func revealStep(w store.Word) *ReviewStep {
	var b strings.Builder
	fmt.Fprintf(&b, "%s — %s", w.Term, w.Translation)
	if w.Note != "" {
		b.WriteString("\n\n" + w.Note)
	}
	b.WriteString("\n\nКак получилось?")
	return &ReviewStep{
		Text: b.String(),
		Actions: []ReviewAction{
			{Data: reviewAgain, Label: "Не помню"},
			{Data: reviewHard, Label: "Сложно"},
			{Data: reviewGood, Label: "Помню"},
			{Data: reviewEasy, Label: "Легко"},
		},
	}
}

// schedule updates a word's SR state following a simplified SM-2 algorithm.
func schedule(w *store.Word, quality int, now time.Time) {
	if quality < 3 {
		w.Reps = 0
		w.Interval = 1
	} else {
		switch w.Reps {
		case 0:
			w.Interval = 1
		case 1:
			w.Interval = 6
		default:
			w.Interval = int(math.Round(float64(w.Interval) * w.Ease))
			if w.Interval < 1 {
				w.Interval = 1
			}
		}
		w.Reps++
	}

	ease := w.Ease
	if ease == 0 {
		ease = defaultEase
	}
	ease += 0.1 - float64(5-quality)*(0.08+float64(5-quality)*0.02)
	if ease < 1.3 {
		ease = 1.3
	}
	w.Ease = ease
	w.DueAt = now.AddDate(0, 0, w.Interval)
}

func qualityOf(data string) (int, bool) {
	switch data {
	case reviewAgain:
		return 1, true
	case reviewHard:
		return 3, true
	case reviewGood:
		return 4, true
	case reviewEasy:
		return 5, true
	default:
		return 0, false
	}
}

func isDue(w store.Word, now time.Time) bool {
	return w.DueAt.IsZero() || !w.DueAt.After(now)
}

func dueIndex(sess *store.Session, now time.Time) int {
	for i, w := range sess.Progress.SavedWords {
		if isDue(w, now) {
			return i
		}
	}
	return -1
}

func countDue(sess *store.Session, now time.Time) int {
	n := 0
	for _, w := range sess.Progress.SavedWords {
		if isDue(w, now) {
			n++
		}
	}
	return n
}

func wordIndex(sess *store.Session, term string) int {
	if term == "" {
		return -1
	}
	for i, w := range sess.Progress.SavedWords {
		if w.Term == term {
			return i
		}
	}
	return -1
}
