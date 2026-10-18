// Command vibetutor runs the Telegram language-tutor bot.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"vibetutor/internal/ai"
	"vibetutor/internal/bot"
	"vibetutor/internal/config"
	"vibetutor/internal/store"
	"vibetutor/internal/tutor"
)

func main() {
	configPath := flag.String("config", "", "path to the config file (default: $VIBETUTOR_CONFIG, else config.txt)")
	flag.Parse()

	logger := log.New(os.Stdout, "", log.LstdFlags)

	path := resolveConfigPath(*configPath)
	if path == "" {
		logger.Print("no config file found, using environment variables and defaults")
	}

	cfg, err := config.Load(path)
	if err != nil {
		logger.Fatalf("load config: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		logger.Fatalf("invalid config: %v", err)
	}

	st, err := store.NewSQLite(cfg.Database)
	if err != nil {
		logger.Fatalf("open store: %v", err)
	}
	defer st.Close()

	model := ai.NewOpenAI(cfg.OpenAIBaseURL, cfg.OpenAIAPIKey, cfg.OpenAIModel, cfg.OpenAITimeout)
	tut := tutor.New(model, st)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	b, err := bot.New(ctx, cfg, tut, logger)
	if err != nil {
		logger.Fatalf("create bot: %v", err)
	}

	go func() {
		<-ctx.Done()
		logger.Print("shutting down")
		b.Stop()
	}()

	b.Run()
}

// resolveConfigPath picks the config file to use. An explicit flag or the
// VIBETUTOR_CONFIG variable is always respected; otherwise config.txt is used
// only when it exists, letting the bot run from environment variables alone.
func resolveConfigPath(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if env := os.Getenv("VIBETUTOR_CONFIG"); env != "" {
		return env
	}
	if _, err := os.Stat("config.txt"); err == nil {
		return "config.txt"
	}
	return ""
}
