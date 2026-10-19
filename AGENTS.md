# AGENTS.md

Инструкции для агентов, работающих с этим репозиторием.

## Что это

Телеграм-бот — репетитор иностранного языка (аналог скилла
[vibeling-tutor](https://github.com/pavlenkovit/vibeling-tutor/blob/main/CLAUDE.md)).
Модель подключается через любой OpenAI-совместимый API.

## Требования

- Go 1.26+
- Внешние зависимости: `gopkg.in/telebot.v4` (Telegram),
  `github.com/glebarez/sqlite` + `gorm.io/gorm` (SQLite, чистый Go).
- Для OpenAI API используется стандартный `net/http`, SDK не добавлять.

## Команды

```bash
make build    # собрать bin/vibetutor
make run      # запустить с config.txt
make test     # go test ./...
make race     # go test -race ./...
make vet      # go vet ./...
make fmt      # gofmt -l -w .
```

Перед завершением задачи обязательно выполнить `gofmt -l .`, `go vet ./...`
и `go test ./...` (для правок с конкурентностью — `go test -race ./...`).
`gofmt -l .` должен печатать пусто.

В этом окружении модуль подключён к `/home/binque/go/src/go.work`. Если
`go`-команды не видят модуль, добавь `./vibetutor` в тот `go.work` или
запускай с `GOWORK=off`.

## Архитектура

```
cmd/vibetutor        точка входа: флаги, сборка зависимостей, graceful shutdown
internal/config      TOML-конфиг + переопределение через VIBETUTOR_* env
internal/ai          клиент OpenAI-совместимого chat completions (net/http)
internal/store       хранилище состояния по пользователям в SQLite (GORM)
internal/tutor       педагогика: setup, plan, lesson, save-words, chat + промпты
internal/bot         адаптер telebot.v4: команды, сообщения, разбивка ответов
```

Поток данных: `bot` -> `tutor` -> (`ai` + `store`). Telegram-специфика не
должна протекать в `tutor`; работа с моделью не должна протекать в `bot`.

## Конвенции

- Принципы DRY и YAGNI: не добавлять абстракции и пакеты «на будущее».
- Держать архитектуру максимально простой; новые зависимости — только при
  явной необходимости.
- Комментарии — минимальные, только для неочевидных мест (godoc на экспорте).
- Промпты и текст общения с пользователем — в `internal/tutor/prompts.go` и
  константами рядом с логикой, на русском.
- Конфиг: сначала TOML-файл, затем env переопределяет. Новые ключи добавлять
  одновременно в `Config`, `set()`, `applyEnv()` и `config.example.txt`.

## Тесты

- Каждый пакет покрывается `*_test.go` рядом.
- Модель подменяется фейком, реализующим `ai.Client` (см. `tutor_test.go`),
  HTTP-сервер — через `net/http/httptest` (см. `openai_test.go`).
- Хранилище в тестах — `store.NewSQLite(filepath.Join(t.TempDir(), "test.db"))`.
- Тестировать чистые функции и поведение, не гоняться за сетью Telegram.

## Git

- Коммиты — Conventional Commits (`feat:`, `fix:`, `docs:`, `chore:`), по
  одному логическому изменению.
- Не коммитить `config.txt`, `.env` и `data/` (они в `.gitignore`).
- Коммиты создавать только по явной просьбе.

## Запуск

- Бинарник: `./bin/vibetutor -config config.txt`.
- Путь конфига: флаг `-config` -> `VIBETUTOR_CONFIG` -> `config.txt`, если
  существует, иначе только env/значения по умолчанию.
- Docker: `docker build -t vibetutor .` или `docker compose up -d --build`.
  В контейнере база — в томе `/data` (`VIBETUTOR_DATABASE=/data/vibetutor.db`).
