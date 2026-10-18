// Package config loads the bot configuration from a simple text file and
// allows overriding every value with environment variables.
package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Default values used when neither the config file nor the environment
// provide a value.
const (
	DefaultBaseURL  = "https://api.openai.com/v1"
	DefaultModel    = "gpt-4o-mini"
	DefaultTimeout  = 60 * time.Second
	DefaultDatabase = "data/vibetutor.db"
)

// Config holds everything the bot needs to run.
type Config struct {
	TelegramToken string
	// AllowedUsers, when non-empty, restricts access to the listed Telegram
	// user IDs. An empty slice means "everyone is allowed".
	AllowedUsers []int64

	OpenAIBaseURL string
	OpenAIAPIKey  string
	OpenAIModel   string
	OpenAITimeout time.Duration

	// Database is the path to the SQLite database file.
	Database string
}

// Default returns a Config populated with sensible defaults.
func Default() *Config {
	return &Config{
		OpenAIBaseURL: DefaultBaseURL,
		OpenAIModel:   DefaultModel,
		OpenAITimeout: DefaultTimeout,
		Database:      DefaultDatabase,
	}
}

// Load reads a key=value text file (if path is non-empty) and then applies
// environment overrides. Lines starting with '#' and blank lines are ignored.
func Load(path string) (*Config, error) {
	cfg := Default()

	if path != "" {
		if err := cfg.loadFile(path); err != nil {
			return nil, err
		}
	}
	cfg.applyEnv()
	return cfg, nil
}

func (c *Config) loadFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open config %q: %w", path, err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	line := 0
	for sc.Scan() {
		line++
		key, value, ok := parseLine(sc.Text())
		if !ok {
			continue
		}
		if err := c.set(key, value); err != nil {
			return fmt.Errorf("config %q line %d: %w", path, line, err)
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("read config %q: %w", path, err)
	}
	return nil
}

// parseLine splits a "key = value" line. It returns ok=false for blank lines
// and comments.
func parseLine(raw string) (key, value string, ok bool) {
	line := strings.TrimSpace(raw)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	k, v, found := strings.Cut(line, "=")
	if !found {
		k, v, found = strings.Cut(line, ":")
	}
	if !found {
		return strings.TrimSpace(line), "", true
	}
	return strings.TrimSpace(k), strings.TrimSpace(v), true
}

func (c *Config) set(key, value string) error {
	switch strings.ToLower(key) {
	case "telegram_token", "telegramtoken":
		c.TelegramToken = value
	case "allowed_users", "allowedusers":
		ids, err := parseIDs(value)
		if err != nil {
			return err
		}
		c.AllowedUsers = ids
	case "openai_base_url", "openai_baseurl", "base_url", "baseurl":
		c.OpenAIBaseURL = value
	case "openai_api_key", "openai_apikey", "api_key", "apikey":
		c.OpenAIAPIKey = value
	case "openai_model", "openai_model_name", "model":
		c.OpenAIModel = value
	case "openai_timeout", "timeout":
		d, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("invalid timeout %q: %w", value, err)
		}
		c.OpenAITimeout = d
	case "database", "database_path", "db":
		c.Database = value
	default:
		return fmt.Errorf("unknown key %q", key)
	}
	return nil
}

func parseIDs(value string) ([]int64, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == ' '
	})
	ids := make([]int64, 0, len(parts))
	for _, p := range parts {
		id, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid user id %q: %w", p, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (c *Config) applyEnv() {
	if v := os.Getenv("VIBETUTOR_TELEGRAM_TOKEN"); v != "" {
		c.TelegramToken = v
	}
	if v := os.Getenv("VIBETUTOR_OPENAI_API_KEY"); v != "" {
		c.OpenAIAPIKey = v
	}
	if v := os.Getenv("VIBETUTOR_OPENAI_BASE_URL"); v != "" {
		c.OpenAIBaseURL = v
	}
	if v := os.Getenv("VIBETUTOR_OPENAI_MODEL"); v != "" {
		c.OpenAIModel = v
	}
	if v := os.Getenv("VIBETUTOR_DATABASE"); v != "" {
		c.Database = v
	}
	if v := os.Getenv("VIBETUTOR_OPENAI_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.OpenAITimeout = d
		}
	}
	if v := os.Getenv("VIBETUTOR_ALLOWED_USERS"); v != "" {
		if ids, err := parseIDs(v); err == nil {
			c.AllowedUsers = ids
		}
	}
}

// Validate reports whether the configuration contains the mandatory values.
func (c *Config) Validate() error {
	if strings.TrimSpace(c.TelegramToken) == "" {
		return fmt.Errorf("telegram token is required")
	}
	if strings.TrimSpace(c.OpenAIBaseURL) == "" {
		return fmt.Errorf("openai base url is required")
	}
	if strings.TrimSpace(c.OpenAIModel) == "" {
		return fmt.Errorf("openai model is required")
	}
	if strings.TrimSpace(c.Database) == "" {
		return fmt.Errorf("database path is required")
	}
	return nil
}

// IsAllowed reports whether the given user is permitted to use the bot.
func (c *Config) IsAllowed(userID int64) bool {
	if len(c.AllowedUsers) == 0 {
		return true
	}
	for _, id := range c.AllowedUsers {
		if id == userID {
			return true
		}
	}
	return false
}
