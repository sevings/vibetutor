// Package store persists per-user tutor state (profile, lesson plan,
// progress and conversation history).
package store

import (
	"time"

	"vibetutor/internal/ai"
)

// Modes a user session can be in.
const (
	ModeChat   = "chat"
	ModeSetup  = "setup"
	ModeLesson = "lesson"
)

// Profile describes what and how a student wants to learn.
type Profile struct {
	Native      string `json:"native,omitempty"`
	Learning    string `json:"learning,omitempty"`
	Explanation string `json:"explanation,omitempty"`
	Level       string `json:"level,omitempty"`
	Goal        string `json:"goal,omitempty"`
}

// Complete reports whether every profile field has been filled in.
func (p Profile) Complete() bool {
	return p.Native != "" && p.Learning != "" && p.Explanation != "" && p.Level != "" && p.Goal != ""
}

// Word is a vocabulary item with its spaced-repetition schedule.
type Word struct {
	Term        string `json:"term"`
	Translation string `json:"translation"`
	Note        string `json:"note,omitempty"`

	// Spaced-repetition state (SM-2). Zero values mean "never reviewed".
	Ease     float64   `json:"ease,omitempty"`
	Interval int       `json:"interval,omitempty"`
	Reps     int       `json:"reps,omitempty"`
	DueAt    time.Time `json:"due_at,omitempty"`
}

// Progress tracks the student's achievements.
type Progress struct {
	CompletedLessons []int  `json:"completed_lessons,omitempty"`
	SavedWords       []Word `json:"saved_words,omitempty"`
}

// Session is the full persisted state of a single Telegram user.
type Session struct {
	Profile          Profile      `json:"profile"`
	Plan             string       `json:"plan,omitempty"`
	LastLessonOutput string       `json:"last_lesson_output,omitempty"`
	History          []ai.Message `json:"history,omitempty"`
	Progress         Progress     `json:"progress"`

	// Runtime routing state.
	Mode         string `json:"mode,omitempty"`
	SetupStep    int    `json:"setup_step,omitempty"`
	LessonNumber int    `json:"lesson_number,omitempty"`

	// ReviewTerm is the word currently being reviewed.
	ReviewTerm string `json:"review_term,omitempty"`

	// ReviewGen identifies the current review session. It is bumped whenever a
	// new session starts so buttons from an older session are rejected.
	ReviewGen int `json:"review_gen,omitempty"`
}

// IsSetup reports whether the user is currently answering setup questions.
func (s *Session) IsSetup() bool { return s.Mode == ModeSetup }

// InLesson reports whether the user is currently inside a lesson.
func (s *Session) InLesson() bool { return s.Mode == ModeLesson }

// NextLesson returns the next lesson number based on the completed lessons.
func (s *Session) NextLesson() int {
	max := 0
	for _, n := range s.Progress.CompletedLessons {
		if n > max {
			max = n
		}
	}
	return max + 1
}

// Store loads and saves user sessions.
type Store interface {
	Load(userID int64) (*Session, error)
	Save(userID int64, s *Session) error
}
