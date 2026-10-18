package tutor

import (
	"fmt"
	"math"
	"strconv"
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

// ReviewStep is one screen of a review session. Stale marks a screen produced
// by a button press that no longer matches the armed word; the transport should
// surface it without editing the active message.
type ReviewStep struct {
	Text    string
	Actions []ReviewAction
	Stale   bool
}

// Review starts (or resumes) a review session and returns the first screen.
func (t *Tutor) Review(userID int64) (*ReviewStep, error) {
	defer t.lock(userID)()
	sess, err := t.store.Load(userID)
	if err != nil {
		return nil, err
	}
	if len(sess.Progress.SavedWords) == 0 {
		return &ReviewStep{Text: "Словарь пуст. Пройди урок и сохрани слова командой /save."}, nil
	}

	step := t.nextReviewStep(sess)
	if err := t.store.Save(userID, sess); err != nil {
		return nil, err
	}
	return step, nil
}

// ReviewAction handles a button press during a review session.
func (t *Tutor) ReviewAction(userID int64, data string) (*ReviewStep, error) {
	defer t.lock(userID)()
	sess, err := t.store.Load(userID)
	if err != nil {
		return nil, err
	}

	code, idx, ok := decodeAction(data)
	if !ok {
		return nil, fmt.Errorf("неизвестное действие повторения %q", data)
	}
	quality, isRating := qualityOf(code)
	if code != reviewShow && !isRating {
		return nil, fmt.Errorf("неизвестное действие повторения %q", data)
	}
	if !reviewIndexValid(sess, idx) {
		return staleReviewStep(), nil
	}

	if code == reviewShow {
		return revealStep(sess.Progress.SavedWords[idx], idx), nil
	}

	schedule(&sess.Progress.SavedWords[idx], quality, time.Now())
	sess.ReviewTerm = ""

	step := t.nextReviewStep(sess)
	if err := t.store.Save(userID, sess); err != nil {
		return nil, err
	}
	return step, nil
}

// nextReviewStep selects the next due word and arms the session for it.
func (t *Tutor) nextReviewStep(sess *store.Session) *ReviewStep {
	now := time.Now()
	idx := dueIndex(sess, now)
	if idx < 0 {
		sess.ReviewTerm = ""
		return &ReviewStep{Text: "Повторение завершено! Слов к повторению больше нет."}
	}

	sess.ReviewTerm = sess.Progress.SavedWords[idx].Term
	pending := countDue(sess, now)
	return &ReviewStep{
		Text: fmt.Sprintf("Слово: %s\n\nВспомни перевод и нажми «Показать». К повторению: %d.", sess.ReviewTerm, pending),
		Actions: []ReviewAction{
			{Data: encodeAction(reviewShow, idx), Label: "Показать"},
		},
	}
}

// revealStep shows the translation and the self-assessment buttons.
func revealStep(w store.Word, idx int) *ReviewStep {
	var b strings.Builder
	fmt.Fprintf(&b, "%s — %s", w.Term, w.Translation)
	if w.Note != "" {
		b.WriteString("\n\n" + w.Note)
	}
	b.WriteString("\n\nКак получилось?")
	return &ReviewStep{
		Text: b.String(),
		Actions: []ReviewAction{
			{Data: encodeAction(reviewAgain, idx), Label: "Не помню"},
			{Data: encodeAction(reviewHard, idx), Label: "Сложно"},
			{Data: encodeAction(reviewGood, idx), Label: "Помню"},
			{Data: encodeAction(reviewEasy, idx), Label: "Легко"},
		},
	}
}

// staleReviewStep is returned when a button no longer matches the armed word,
// e.g. when an older review message is tapped after the session moved on.
func staleReviewStep() *ReviewStep {
	return &ReviewStep{
		Text:  "Это сообщение устарело. Продолжай в последнем сообщении с повторением.",
		Stale: true,
	}
}

// reviewIndexValid reports whether idx points to the word currently armed in
// the session, guarding against stale buttons from older messages.
func reviewIndexValid(sess *store.Session, idx int) bool {
	return sess.ReviewTerm != "" &&
		idx >= 0 && idx < len(sess.Progress.SavedWords) &&
		sess.Progress.SavedWords[idx].Term == sess.ReviewTerm
}

// encodeAction packs an action code and the word index into callback data.
func encodeAction(code string, idx int) string {
	return code + ":" + strconv.Itoa(idx)
}

// decodeAction splits callback data back into an action code and word index.
func decodeAction(data string) (string, int, bool) {
	i := strings.LastIndex(data, ":")
	if i < 0 {
		return "", 0, false
	}
	idx, err := strconv.Atoi(data[i+1:])
	if err != nil {
		return "", 0, false
	}
	return data[:i], idx, true
}

// schedule updates a word's SR state following a simplified SM-2 algorithm.
func schedule(w *store.Word, quality int, now time.Time) {
	ease := w.Ease
	if ease == 0 {
		ease = defaultEase
	}

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
			w.Interval = int(math.Round(float64(w.Interval) * ease))
			if w.Interval < 1 {
				w.Interval = 1
			}
		}
		w.Reps++
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
