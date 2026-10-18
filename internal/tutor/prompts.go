package tutor

import (
	"fmt"
	"strings"

	"vibetutor/internal/store"
)

const persona = "Ты — терпеливый репетитор иностранного языка. Ученик проходит уроки в чате."

const rules = `Правила:
- Объясняй на языке объяснений, примеры давай на изучаемом языке.
- Коротко: правило в двух-трёх фразах, затем примеры, затем практика.
- Одно задание за раз. Дождись ответа, проверь, объясни ошибку и двигайся дальше.
- Не выдавай ответы заранее. Ошибка — не повод для лекции: исправь и дай похожее задание.
- Уровень сложности — по профилю, не забегай вперёд плана.
- Пиши простым текстом, без Markdown-разметки.`

// profileBlock renders the student's profile for the system prompt.
func profileBlock(p store.Profile) string {
	return fmt.Sprintf(`Профиль ученика:
- родной язык: %s
- изучаемый язык: %s
- язык объяснений: %s
- уровень: %s
- цель: %s`, p.Native, p.Learning, p.Explanation, p.Level, p.Goal)
}

// chatSystem builds the system prompt for free-form conversation.
func chatSystem(sess *store.Session) string {
	parts := []string{persona, profileBlock(sess.Profile), rules}
	if sess.Plan != "" {
		parts = append(parts, "План уроков:\n"+sess.Plan)
	}
	return strings.Join(parts, "\n\n")
}

// planSystem builds the system prompt used to create a lesson plan.
func planSystem(sess *store.Session) string {
	return strings.Join([]string{
		persona,
		profileBlock(sess.Profile),
		`Составь подробный план уроков для этого ученика. Для каждого урока укажи номер, тему и краткое содержание. Нумеруй уроки с 1. Отвечай простым текстом.`,
	}, "\n\n")
}

// lessonSystem builds the system prompt used while running a lesson.
func lessonSystem(sess *store.Session) string {
	parts := []string{persona, profileBlock(sess.Profile), rules}
	if sess.Plan != "" {
		parts = append(parts, "План уроков:\n"+sess.Plan)
	}
	if done := formatLessons(sess.Progress.CompletedLessons); done != "" {
		parts = append(parts, "Пройденные уроки: "+done)
	}
	if len(sess.Progress.SavedWords) > 0 {
		parts = append(parts, fmt.Sprintf("Уже выучено слов: %d.", len(sess.Progress.SavedWords)))
	}
	parts = append(parts, fmt.Sprintf(
		"Сейчас идёт урок номер %d. Веди урок по шагам: короткое объяснение, примеры, практика. Жди ответов ученика.",
		sess.LessonNumber))
	return strings.Join(parts, "\n\n")
}

// saveWordsSystem instructs the model to extract vocabulary as JSON.
func saveWordsSystem() string {
	return `Ты извлекаешь словарь из текста урока иностранного языка.
Верни ТОЛЬКО JSON-массив без пояснений и markdown. Формат:
[{"term":"слово на изучаемом языке","translation":"перевод на язык объяснений","note":"краткая заметка"}]
Поле note необязательно. Включай только действительно изучаемые слова и выражения.`
}

func formatLessons(lessons []int) string {
	if len(lessons) == 0 {
		return ""
	}
	parts := make([]string, len(lessons))
	for i, n := range lessons {
		parts[i] = fmt.Sprintf("%d", n)
	}
	return strings.Join(parts, ", ")
}
