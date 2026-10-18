package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"vibetutor/internal/ai"
)

// SQLiteStore persists user sessions in a SQLite database.
type SQLiteStore struct {
	db *gorm.DB
}

// record is the database representation of a Session.
type record struct {
	UserID int64 `gorm:"primaryKey;autoIncrement:false"`

	Native      string
	Learning    string
	Explanation string
	Level       string
	Goal        string

	Plan             string
	LastLessonOutput string

	History          []ai.Message `gorm:"serializer:json"`
	CompletedLessons []int        `gorm:"serializer:json"`
	Difficult        []string     `gorm:"serializer:json"`
	SavedWords       []Word       `gorm:"serializer:json"`

	Mode         string
	SetupStep    int
	LessonNumber int
	ReviewTerm   string

	UpdatedAt time.Time
}

// NewSQLite opens (creating if needed) the database at path and runs the
// schema migration.
func NewSQLite(path string) (*SQLiteStore, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create database dir %q: %w", dir, err)
		}
	}

	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open database %q: %w", path, err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("access database handle: %w", err)
	}
	// A single connection avoids SQLITE_BUSY between concurrent writers and
	// keeps the connection-local pragmas effective.
	sqlDB.SetMaxOpenConns(1)
	db.Exec("PRAGMA journal_mode=WAL")
	db.Exec("PRAGMA busy_timeout=5000")

	if err := db.AutoMigrate(&record{}); err != nil {
		return nil, fmt.Errorf("migrate database: %w", err)
	}
	return &SQLiteStore{db: db}, nil
}

// Load returns the stored session, or a fresh one when the user is new.
func (s *SQLiteStore) Load(userID int64) (*Session, error) {
	var r record
	err := s.db.First(&r, "user_id = ?", userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &Session{Mode: ModeChat}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load session %d: %w", userID, err)
	}
	return r.toSession(), nil
}

// Save inserts or updates the user's session.
func (s *SQLiteStore) Save(userID int64, sess *Session) error {
	r := toRecord(userID, sess)
	if err := s.db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&r).Error; err != nil {
		return fmt.Errorf("save session %d: %w", userID, err)
	}
	return nil
}

// Close releases the underlying database connection.
func (s *SQLiteStore) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func toRecord(userID int64, sess *Session) *record {
	return &record{
		UserID:           userID,
		Native:           sess.Profile.Native,
		Learning:         sess.Profile.Learning,
		Explanation:      sess.Profile.Explanation,
		Level:            sess.Profile.Level,
		Goal:             sess.Profile.Goal,
		Plan:             sess.Plan,
		LastLessonOutput: sess.LastLessonOutput,
		History:          sess.History,
		CompletedLessons: sess.Progress.CompletedLessons,
		Difficult:        sess.Progress.Difficult,
		SavedWords:       sess.Progress.SavedWords,
		Mode:             sess.Mode,
		SetupStep:        sess.SetupStep,
		LessonNumber:     sess.LessonNumber,
		ReviewTerm:       sess.ReviewTerm,
	}
}

func (r *record) toSession() *Session {
	return &Session{
		Profile: Profile{
			Native:      r.Native,
			Learning:    r.Learning,
			Explanation: r.Explanation,
			Level:       r.Level,
			Goal:        r.Goal,
		},
		Plan:             r.Plan,
		LastLessonOutput: r.LastLessonOutput,
		History:          r.History,
		Progress: Progress{
			CompletedLessons: r.CompletedLessons,
			Difficult:        r.Difficult,
			SavedWords:       r.SavedWords,
		},
		Mode:         r.Mode,
		SetupStep:    r.SetupStep,
		LessonNumber: r.LessonNumber,
		ReviewTerm:   r.ReviewTerm,
	}
}
