// Package tutor implements the teaching logic: profile setup, lesson
// planning, running lessons and saving vocabulary.
package tutor

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"vibetutor/internal/ai"
	"vibetutor/internal/store"
)

// maxHistory is the number of recent conversation messages kept per user.
const maxHistory = 24

// Messages exposed to the transport layer.
const (
	MsgNeedSetup = "Сначала настрой профиль командой /setup."
	MsgNeedPlan  = "Плана уроков пока нет. Создай его командой /plan."
)

var setupQuestions = []string{
	"Какой у тебя родной язык?",
	"Какой язык хочешь учить?",
	"На каком языке объяснять материал?",
	"Какой у тебя текущий уровень? (например: начинающий, A2, B1)",
	"Какая у тебя цель? (например: свободно говорить, сдать экзамен)",
}

// Tutor ties the language model and the user store together.
type Tutor struct {
	ai    ai.Client
	store store.Store
}

// New returns a Tutor.
func New(client ai.Client, s store.Store) *Tutor {
	return &Tutor{ai: client, store: s}
}

// Welcome is the message shown on /start.
func (t *Tutor) Welcome() string {
	return "Привет! Я репетитор иностранного языка.\n\n" +
		"Сначала настрой профиль командой /setup — я спрошу о языках, уровне и цели.\n" +
		"Затем составь план командой /plan и начинай уроки командой /lesson.\n\n" +
		"Список команд: /help"
}

// Help describes the available commands.
func (t *Tutor) Help() string {
	return strings.Join([]string{
		"Команды:",
		"/setup — настроить профиль ученика",
		"/profile — показать профиль",
		"/plan — составить план уроков",
		"/lesson — начать следующий урок (/lesson 3 — конкретный)",
		"/save — сохранить слова последнего урока",
		"/review — повторять сохранённые слова (интервальное повторение)",
		"/reset — сбросить профиль, план и прогресс",
		"/help — эта справка",
		"",
		"Любое обычное сообщение во время урока — это твой ответ репетитору.",
	}, "\n")
}

// Setup (re)starts the profile questionnaire.
func (t *Tutor) Setup(_ context.Context, userID int64) (string, error) {
	sess, err := t.store.Load(userID)
	if err != nil {
		return "", err
	}
	sess.Profile = store.Profile{}
	sess.Mode = store.ModeSetup
	sess.SetupStep = 0
	if err := t.store.Save(userID, sess); err != nil {
		return "", err
	}
	return "Настроим профиль. " + setupQuestions[0], nil
}

// Profile renders the current profile.
func (t *Tutor) Profile(_ context.Context, userID int64) (string, error) {
	sess, err := t.store.Load(userID)
	if err != nil {
		return "", err
	}
	if !sess.Profile.Complete() {
		return MsgNeedSetup, nil
	}
	p := sess.Profile
	return fmt.Sprintf("Профиль:\n- родной язык: %s\n- изучаемый язык: %s\n- язык объяснений: %s\n- уровень: %s\n- цель: %s",
		p.Native, p.Learning, p.Explanation, p.Level, p.Goal), nil
}

// Reset clears all stored data for the user.
func (t *Tutor) Reset(_ context.Context, userID int64) (string, error) {
	if err := t.store.Save(userID, &store.Session{Mode: store.ModeChat}); err != nil {
		return "", err
	}
	return "Готово. Профиль, план и прогресс сброшены. Начни заново командой /setup.", nil
}

// Handle processes a free-form message according to the current mode.
func (t *Tutor) Handle(ctx context.Context, userID int64, text string) (string, error) {
	sess, err := t.store.Load(userID)
	if err != nil {
		return "", err
	}
	if sess.IsSetup() {
		return t.continueSetup(userID, sess, text)
	}
	if !sess.Profile.Complete() {
		return MsgNeedSetup, nil
	}
	return t.reply(ctx, userID, sess, text)
}

// continueSetup records one answer and asks the next question.
func (t *Tutor) continueSetup(userID int64, sess *store.Session, answer string) (string, error) {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return "Пожалуйста, напиши ответ текстом.", nil
	}
	setProfileField(&sess.Profile, sess.SetupStep, answer)
	sess.SetupStep++

	if sess.SetupStep < len(setupQuestions) {
		if err := t.store.Save(userID, sess); err != nil {
			return "", err
		}
		return setupQuestions[sess.SetupStep], nil
	}

	sess.Mode = store.ModeChat
	if err := t.store.Save(userID, sess); err != nil {
		return "", err
	}
	return fmt.Sprintf("Профиль сохранён!\n%s\n\nТеперь составь план уроков командой /plan.", t.mustProfile(sess)), nil
}

// Plan generates a lesson plan with the model.
func (t *Tutor) Plan(ctx context.Context, userID int64) (string, error) {
	sess, err := t.store.Load(userID)
	if err != nil {
		return "", err
	}
	if !sess.Profile.Complete() {
		return MsgNeedSetup, nil
	}

	reply, err := t.ai.Complete(ctx, []ai.Message{
		{Role: ai.RoleSystem, Content: planSystem(sess)},
		{Role: ai.RoleUser, Content: "Составь план уроков."},
	})
	if err != nil {
		return "", err
	}

	sess.Plan = reply
	sess.Mode = store.ModeChat
	sess.History = nil
	if err := t.store.Save(userID, sess); err != nil {
		return "", err
	}
	return reply, nil
}

// Lesson starts a lesson, either the given number or the next one.
func (t *Tutor) Lesson(ctx context.Context, userID int64, arg string) (string, error) {
	sess, err := t.store.Load(userID)
	if err != nil {
		return "", err
	}
	if !sess.Profile.Complete() {
		return MsgNeedSetup, nil
	}
	if sess.Plan == "" {
		return MsgNeedPlan, nil
	}

	number := sess.NextLesson()
	if s := strings.TrimSpace(arg); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 {
			return "Номер урока должен быть положительным числом, например /lesson 3.", nil
		}
		number = n
	}

	sess.Mode = store.ModeLesson
	sess.LessonNumber = number
	sess.History = nil

	user := fmt.Sprintf("Начнём урок номер %d.", number)
	reply, err := t.ai.Complete(ctx, []ai.Message{
		{Role: ai.RoleSystem, Content: lessonSystem(sess)},
		{Role: ai.RoleUser, Content: user},
	})
	if err != nil {
		return "", err
	}

	sess.LastLessonOutput = reply
	sess.History = []ai.Message{
		{Role: ai.RoleUser, Content: user},
		{Role: ai.RoleAssistant, Content: reply},
	}
	if err := t.store.Save(userID, sess); err != nil {
		return "", err
	}
	return reply, nil
}

// reply continues the ongoing chat or lesson conversation.
func (t *Tutor) reply(ctx context.Context, userID int64, sess *store.Session, text string) (string, error) {
	system := chatSystem(sess)
	if sess.InLesson() {
		system = lessonSystem(sess)
	}

	messages := make([]ai.Message, 0, len(sess.History)+2)
	messages = append(messages, ai.Message{Role: ai.RoleSystem, Content: system})
	messages = append(messages, sess.History...)
	messages = append(messages, ai.Message{Role: ai.RoleUser, Content: text})

	reply, err := t.ai.Complete(ctx, messages)
	if err != nil {
		return "", err
	}

	sess.History = append(sess.History,
		ai.Message{Role: ai.RoleUser, Content: text},
		ai.Message{Role: ai.RoleAssistant, Content: reply},
	)
	sess.History = trimHistory(sess.History)

	if sess.InLesson() {
		sess.LastLessonOutput = reply
	}
	if err := t.store.Save(userID, sess); err != nil {
		return "", err
	}
	return reply, nil
}

// SaveWords extracts vocabulary from the last lesson and stores it.
func (t *Tutor) SaveWords(ctx context.Context, userID int64) (string, error) {
	sess, err := t.store.Load(userID)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(sess.LastLessonOutput) == "" {
		return "Пока нечего сохранять: сначала пройди урок командой /lesson.", nil
	}

	raw, err := t.ai.Complete(ctx, []ai.Message{
		{Role: ai.RoleSystem, Content: saveWordsSystem()},
		{Role: ai.RoleUser, Content: sess.LastLessonOutput},
	})
	if err != nil {
		return "", err
	}

	words, err := parseWords(raw)
	if err != nil {
		return "", fmt.Errorf("не удалось разобрать список слов, попробуй ещё раз: %w", err)
	}
	added := addWords(&sess.Progress, words)

	if sess.LessonNumber > 0 {
		markLessonDone(&sess.Progress, sess.LessonNumber)
	}
	sess.Mode = store.ModeChat

	if err := t.store.Save(userID, sess); err != nil {
		return "", err
	}
	if added == 0 {
		return "Новых слов не нашлось. Урок отмечен как пройденный.", nil
	}
	return fmt.Sprintf("Сохранил %d новых слов. Всего в словаре: %d.", added, len(sess.Progress.SavedWords)), nil
}

func (t *Tutor) mustProfile(sess *store.Session) string {
	p := sess.Profile
	return fmt.Sprintf("Профиль:\n- родной язык: %s\n- изучаемый язык: %s\n- язык объяснений: %s\n- уровень: %s\n- цель: %s",
		p.Native, p.Learning, p.Explanation, p.Level, p.Goal)
}

func setProfileField(p *store.Profile, step int, value string) {
	switch step {
	case 0:
		p.Native = value
	case 1:
		p.Learning = value
	case 2:
		p.Explanation = value
	case 3:
		p.Level = value
	case 4:
		p.Goal = value
	}
}

func trimHistory(h []ai.Message) []ai.Message {
	if len(h) <= maxHistory {
		return h
	}
	return h[len(h)-maxHistory:]
}

// parseWords extracts a JSON array of words from a model reply that may
// contain surrounding text.
func parseWords(raw string) ([]store.Word, error) {
	start := strings.Index(raw, "[")
	end := strings.LastIndex(raw, "]")
	if start == -1 || end == -1 || end < start {
		return nil, fmt.Errorf("в ответе нет JSON-массива")
	}
	var words []store.Word
	if err := json.Unmarshal([]byte(raw[start:end+1]), &words); err != nil {
		return nil, err
	}
	return words, nil
}

// addWords appends words, skipping duplicates, and returns how many were added.
func addWords(p *store.Progress, words []store.Word) int {
	seen := make(map[string]bool, len(p.SavedWords))
	for _, w := range p.SavedWords {
		seen[strings.ToLower(w.Term)] = true
	}
	added := 0
	for _, w := range words {
		key := strings.ToLower(strings.TrimSpace(w.Term))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		p.SavedWords = append(p.SavedWords, w)
		added++
	}
	return added
}

func markLessonDone(p *store.Progress, number int) {
	for _, n := range p.CompletedLessons {
		if n == number {
			return
		}
	}
	p.CompletedLessons = append(p.CompletedLessons, number)
	sort.Ints(p.CompletedLessons)
}
