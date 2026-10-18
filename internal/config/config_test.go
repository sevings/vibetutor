package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.txt")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoadFile(t *testing.T) {
	path := writeConfig(t, `
# comment line
telegram_token = tok123
openai_base_url = http://localhost:11434/v1
openai_api_key = secret
openai_model = llama3
openai_timeout = 30s
database = /tmp/vibetutor.db
allowed_users = 1, 2;3
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.TelegramToken != "tok123" {
		t.Errorf("token = %q", cfg.TelegramToken)
	}
	if cfg.OpenAIBaseURL != "http://localhost:11434/v1" {
		t.Errorf("base url = %q", cfg.OpenAIBaseURL)
	}
	if cfg.OpenAIAPIKey != "secret" {
		t.Errorf("api key = %q", cfg.OpenAIAPIKey)
	}
	if cfg.OpenAIModel != "llama3" {
		t.Errorf("model = %q", cfg.OpenAIModel)
	}
	if cfg.OpenAITimeout != 30*time.Second {
		t.Errorf("timeout = %v", cfg.OpenAITimeout)
	}
	if cfg.Database != "/tmp/vibetutor.db" {
		t.Errorf("database = %q", cfg.Database)
	}
	if len(cfg.AllowedUsers) != 3 || cfg.AllowedUsers[2] != 3 {
		t.Errorf("allowed users = %v", cfg.AllowedUsers)
	}
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.OpenAIBaseURL != DefaultBaseURL || cfg.OpenAIModel != DefaultModel {
		t.Errorf("defaults not applied: %+v", cfg)
	}
	if cfg.OpenAITimeout != DefaultTimeout || cfg.Database != DefaultDatabase {
		t.Errorf("defaults not applied: %+v", cfg)
	}
}

func TestLoadEnvOverride(t *testing.T) {
	path := writeConfig(t, "telegram_token = file\nopenai_model = file-model\n")
	t.Setenv("VIBETUTOR_TELEGRAM_TOKEN", "env-token")
	t.Setenv("VIBETUTOR_OPENAI_MODEL", "env-model")
	t.Setenv("VIBETUTOR_ALLOWED_USERS", "42")
	t.Setenv("VIBETUTOR_DATABASE", "/tmp/env.db")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.TelegramToken != "env-token" {
		t.Errorf("token = %q, want env-token", cfg.TelegramToken)
	}
	if cfg.OpenAIModel != "env-model" {
		t.Errorf("model = %q, want env-model", cfg.OpenAIModel)
	}
	if !cfg.IsAllowed(42) || cfg.IsAllowed(7) {
		t.Errorf("allowed users override failed: %v", cfg.AllowedUsers)
	}
	if cfg.Database != "/tmp/env.db" {
		t.Errorf("database = %q, want /tmp/env.db", cfg.Database)
	}
}

func TestLoadErrors(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		if _, err := Load(filepath.Join(t.TempDir(), "nope.txt")); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("bad timeout", func(t *testing.T) {
		if _, err := Load(writeConfig(t, "openai_timeout = soon")); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("bad user id", func(t *testing.T) {
		if _, err := Load(writeConfig(t, "allowed_users = abc")); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("unknown key", func(t *testing.T) {
		if _, err := Load(writeConfig(t, "mystery = 1")); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("bad env user id", func(t *testing.T) {
		t.Setenv("VIBETUTOR_ALLOWED_USERS", "123, oops")
		if _, err := Load(""); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("bad env timeout", func(t *testing.T) {
		t.Setenv("VIBETUTOR_OPENAI_TIMEOUT", "soon")
		if _, err := Load(""); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestValidate(t *testing.T) {
	cfg := Default()
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error without token")
	}
	cfg.TelegramToken = "x"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestIsAllowed(t *testing.T) {
	cfg := &Config{}
	if !cfg.IsAllowed(1) {
		t.Error("empty allow list should allow everyone")
	}
	cfg.AllowedUsers = []int64{5, 9}
	if cfg.IsAllowed(1) || !cfg.IsAllowed(9) {
		t.Error("allow list filtering failed")
	}
}
