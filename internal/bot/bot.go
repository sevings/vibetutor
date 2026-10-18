// Package bot wires the tutor logic to Telegram using telebot.v4.
package bot

import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	tele "gopkg.in/telebot.v4"

	"vibetutor/internal/config"
	"vibetutor/internal/tutor"
)

// maxMessageLen is a safe chunk size below Telegram's 4096-character limit.
const maxMessageLen = 4000

const notAllowedMsg = "Извини, доступ к этому боту ограничен."

const emptyReplyMsg = "Модель вернула пустой ответ. Попробуй ещё раз."

// reviewButtonUnique is the callback endpoint for review keyboard buttons.
const reviewButtonUnique = "rev"

// commandPattern matches a Telegram command at the start of a message, e.g.
// "/help" or "/lesson@mybot 3". It is used to distinguish unknown commands from
// ordinary text that merely begins with a slash (e.g. a phonetic answer).
var commandPattern = regexp.MustCompile(`^/[A-Za-z0-9_]+(@[A-Za-z0-9_]+)?(\s|$)`)

// Bot is the Telegram adapter around the tutor.
type Bot struct {
	api   *tele.Bot
	tutor *tutor.Tutor
	cfg   *config.Config
	log   *log.Logger
	ctx   context.Context
}

// New constructs the bot. The provided context is used for model calls and
// can be cancelled to abort in-flight requests on shutdown.
func New(ctx context.Context, cfg *config.Config, t *tutor.Tutor, logger *log.Logger) (*Bot, error) {
	if logger == nil {
		logger = log.Default()
	}
	api, err := tele.NewBot(tele.Settings{
		Token:  cfg.TelegramToken,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
		OnError: func(err error, c tele.Context) {
			logger.Printf("telegram error: %v", err)
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create telegram bot: %w", err)
	}
	return &Bot{api: api, tutor: t, cfg: cfg, log: logger, ctx: ctx}, nil
}

// Run registers the handlers, publishes the command menu and starts polling.
func (b *Bot) Run() {
	b.register()
	b.publishCommands()
	b.log.Printf("bot started as @%s", b.api.Me.Username)
	b.api.Start()
}

// Stop gracefully stops the polling loop.
func (b *Bot) Stop() { b.api.Stop() }

func (b *Bot) register() {
	b.api.Handle("/start", b.command(func(context.Context, int64) (string, error) {
		return b.tutor.Welcome(), nil
	}))
	b.api.Handle("/help", b.command(func(context.Context, int64) (string, error) {
		return b.tutor.Help(), nil
	}))
	b.api.Handle("/setup", b.command(b.tutor.Setup))
	b.api.Handle("/profile", b.command(b.tutor.Profile))
	b.api.Handle("/plan", b.command(b.tutor.Plan))
	b.api.Handle("/lesson", b.commandArg(b.tutor.Lesson))
	b.api.Handle("/save", b.command(b.tutor.SaveWords))
	b.api.Handle("/review", b.reviewCommand)
	b.api.Handle(&tele.InlineButton{Unique: reviewButtonUnique}, b.onReviewCallback)
	b.api.Handle("/reset", b.command(b.tutor.Reset))
	b.api.Handle(tele.OnText, b.onText)
}

func (b *Bot) publishCommands() {
	commands := []tele.Command{
		{Text: "start", Description: "Начать работу"},
		{Text: "setup", Description: "Настроить профиль"},
		{Text: "profile", Description: "Показать профиль"},
		{Text: "plan", Description: "Составить план уроков"},
		{Text: "lesson", Description: "Начать урок"},
		{Text: "save", Description: "Сохранить слова урока"},
		{Text: "review", Description: "Повторять слова"},
		{Text: "reset", Description: "Сбросить прогресс"},
		{Text: "help", Description: "Справка"},
	}
	if err := b.api.SetCommands(commands); err != nil {
		b.log.Printf("set commands: %v", err)
	}
}

// command adapts a tutor method without arguments into a telebot handler.
func (b *Bot) command(fn func(ctx context.Context, userID int64) (string, error)) tele.HandlerFunc {
	return b.commandArg(func(ctx context.Context, userID int64, _ string) (string, error) {
		return fn(ctx, userID)
	})
}

// commandArg adapts a tutor method that reads the command argument.
func (b *Bot) commandArg(fn func(ctx context.Context, userID int64, arg string) (string, error)) tele.HandlerFunc {
	return func(c tele.Context) error {
		sender := c.Sender()
		if sender == nil {
			return nil
		}
		if !b.cfg.IsAllowed(sender.ID) {
			return c.Send(notAllowedMsg)
		}
		b.respond(c, func(ctx context.Context) (string, error) {
			return fn(ctx, sender.ID, c.Data())
		})
		return nil
	}
}

func (b *Bot) onText(c tele.Context) error {
	sender := c.Sender()
	if sender == nil {
		return nil
	}
	if !b.cfg.IsAllowed(sender.ID) {
		return c.Send(notAllowedMsg)
	}

	text := c.Text()
	if commandPattern.MatchString(strings.TrimSpace(text)) {
		return c.Send("Неизвестная команда. Список команд: /help")
	}

	b.respond(c, func(ctx context.Context) (string, error) {
		return b.tutor.Handle(ctx, sender.ID, text)
	})
	return nil
}

// respond shows a typing indicator, runs fn and sends the (possibly long)
// reply back to the chat.
func (b *Bot) respond(c tele.Context, fn func(ctx context.Context) (string, error)) {
	if err := c.Notify(tele.Typing); err != nil {
		b.log.Printf("notify typing: %v", err)
	}

	reply, err := fn(b.ctx)
	if err != nil {
		b.log.Printf("handle request: %v", err)
		if sendErr := c.Send("Не получилось обработать запрос. Попробуй ещё раз чуть позже."); sendErr != nil {
			b.log.Printf("send error notice: %v", sendErr)
		}
		return
	}

	for _, chunk := range outgoingChunks(reply) {
		if err := c.Send(chunk); err != nil {
			b.log.Printf("send reply: %v", err)
			return
		}
	}
}

// outgoingChunks splits a model reply into messages, falling back to a notice
// when the reply is empty so the user is not left without a response.
func outgoingChunks(reply string) []string {
	if chunks := SplitMessage(reply, maxMessageLen); len(chunks) > 0 {
		return chunks
	}
	return []string{emptyReplyMsg}
}

// reviewCommand starts a spaced-repetition session.
func (b *Bot) reviewCommand(c tele.Context) error {
	sender := c.Sender()
	if sender == nil {
		return nil
	}
	if !b.cfg.IsAllowed(sender.ID) {
		return c.Send(notAllowedMsg)
	}
	if err := c.Notify(tele.Typing); err != nil {
		b.log.Printf("notify typing: %v", err)
	}

	step, err := b.tutor.Review(sender.ID)
	if err != nil {
		b.log.Printf("start review: %v", err)
		return c.Send("Не получилось начать повторение. Попробуй позже.")
	}
	return b.sendReview(c, step)
}

// onReviewCallback reacts to the review keyboard buttons.
func (b *Bot) onReviewCallback(c tele.Context) error {
	sender := c.Sender()
	if sender == nil {
		return c.Respond()
	}
	if !b.cfg.IsAllowed(sender.ID) {
		return c.Respond(&tele.CallbackResponse{Text: notAllowedMsg})
	}

	step, err := b.tutor.ReviewAction(sender.ID, c.Data())
	if err != nil {
		b.log.Printf("review action: %v", err)
		return c.Respond(&tele.CallbackResponse{Text: "Ошибка. Попробуй ещё раз."})
	}
	if step != nil && step.Stale {
		return c.Respond(&tele.CallbackResponse{Text: step.Text})
	}
	if err := c.Respond(); err != nil {
		b.log.Printf("answer callback: %v", err)
	}
	if step == nil {
		return nil
	}
	if markup := reviewMarkup(step); markup != nil {
		if err := c.Edit(step.Text, markup); err != nil && !isMessageNotModified(err) {
			return err
		}
		return nil
	}
	// No actions left: editing without a markup removes the inline keyboard.
	if err := c.Edit(step.Text); err != nil && !isMessageNotModified(err) {
		return err
	}
	return nil
}

// isMessageNotModified reports whether err is Telegram rejecting a no-op edit,
// which happens when the same button is tapped twice in quick succession.
func isMessageNotModified(err error) bool {
	return errors.Is(err, tele.ErrMessageNotModified) || errors.Is(err, tele.ErrSameMessageContent)
}

// sendReview sends a review screen to the chat, splitting long text and
// attaching the action keyboard to the final chunk.
func (b *Bot) sendReview(c tele.Context, step *tutor.ReviewStep) error {
	if step == nil {
		return nil
	}
	markup := reviewMarkup(step)
	chunks := SplitMessage(step.Text, maxMessageLen)
	if len(chunks) == 0 {
		chunks = []string{emptyReplyMsg}
	}
	for i, chunk := range chunks {
		last := i == len(chunks)-1
		var err error
		if last && markup != nil {
			err = c.Send(chunk, markup)
		} else {
			err = c.Send(chunk)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// reviewMarkup renders the action buttons as an inline keyboard.
func reviewMarkup(step *tutor.ReviewStep) *tele.ReplyMarkup {
	if step == nil || len(step.Actions) == 0 {
		return nil
	}
	markup := &tele.ReplyMarkup{}
	rows := make([][]tele.InlineButton, 0, len(step.Actions))
	for _, a := range step.Actions {
		rows = append(rows, []tele.InlineButton{{
			Unique: reviewButtonUnique,
			Text:   a.Label,
			Data:   a.Data,
		}})
	}
	markup.InlineKeyboard = rows
	return markup
}

// SplitMessage splits text into chunks no longer than limit bytes, trying to
// break on newlines and never splitting a UTF-8 rune.
func SplitMessage(text string, limit int) []string {
	if limit <= 0 {
		limit = maxMessageLen
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if len(text) <= limit {
		return []string{text}
	}

	var chunks []string
	for len(text) > limit {
		cut := limit
		if i := strings.LastIndexByte(text[:limit], '\n'); i > 0 {
			cut = i
		}
		cut = utf8SafeCut(text, cut)
		if cut == 0 {
			// The window is entirely UTF-8 continuation bytes; fall back to
			// the raw limit so the loop always makes progress.
			cut = limit
		}
		if chunk := strings.TrimSpace(text[:cut]); chunk != "" {
			chunks = append(chunks, chunk)
		}
		text = strings.TrimLeft(text[cut:], "\n")
	}
	if chunk := strings.TrimSpace(text); chunk != "" {
		chunks = append(chunks, chunk)
	}
	return chunks
}

// utf8SafeCut moves a byte index back until it is on a rune boundary.
func utf8SafeCut(s string, cut int) int {
	for cut > 0 && cut < len(s) && s[cut]&0xC0 == 0x80 {
		cut--
	}
	return cut
}
