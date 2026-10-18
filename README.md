# Vibetutor

Телеграм-бот — репетитор иностранного языка. Повторяет функциональность
скилла [vibeling-tutor](https://github.com/pavlenkovit/vibeling-tutor/blob/main/CLAUDE.md)
в виде обычного Telegram-бота: настраивает профиль ученика, составляет план
уроков, ведёт уроки и сохраняет выученные слова.

Поддерживается любой OpenAI-совместимый API: OpenAI, локальные Ollama,
LM Studio, vLLM и т. п. Адрес API задаётся в конфиге.

## Возможности

- `/setup` — пошаговая настройка профиля: родной язык, изучаемый язык, язык
  объяснений, уровень, цель.
- `/plan` — план уроков под язык, уровень и цель.
- `/lesson [номер]` — следующий или конкретный урок. Урок идёт как диалог:
  правило → примеры → практика, одно задание за раз.
- `/save` — извлекает слова урока и сохраняет их в личный словарь.
- `/profile`, `/reset`, `/help`, `/start`.
- Прогресс, план, словарь и история переписки хранятся отдельно для каждого
  пользователя в каталоге `data/`.
- Необязательный белый список пользователей.

## Команды бота

| Команда     | Описание                                   |
|-------------|--------------------------------------------|
| `/start`    | Приветствие и краткая инструкция           |
| `/setup`    | Настроить или сбросить профиль             |
| `/profile`  | Показать текущий профиль                   |
| `/plan`     | Составить план уроков                      |
| `/lesson`   | Начать следующий урок (`/lesson 3` — № 3)  |
| `/save`     | Сохранить слова последнего урока           |
| `/reset`    | Сбросить профиль, план и прогресс          |
| `/help`     | Справка                                    |

Любое обычное сообщение во время урока — это ответ ученика репетитору.

## Быстрый старт (исполняемый файл)

Требуется Go 1.26+.

```bash
cp config.example.txt config.txt
# отредактируйте config.txt: токен бота и параметры API
make build          # соберёт bin/vibetutor
./bin/vibetutor -config config.txt
```

Или без сборки:

```bash
go run ./cmd/vibetutor -config config.txt
```

`-config` можно не указывать: тогда используется `config.txt`, если он есть,
иначе только переменные окружения и значения по умолчанию.

## Запуск в Docker

Соберите образ и запустите контейнер, передав параметры переменными
окружения:

```bash
docker build -t vibetutor .
docker run -d --name vibetutor --restart unless-stopped \
  -e VIBETUTOR_TELEGRAM_TOKEN=123456:ABC... \
  -e VIBETUTOR_OPENAI_API_KEY=sk-... \
  -e VIBETUTOR_OPENAI_BASE_URL=https://api.openai.com/v1 \
  -e VIBETUTOR_OPENAI_MODEL=gpt-4o-mini \
  -e VIBETUTOR_ALLOWED_USERS=123456789 \
  -v "$PWD/data:/data" \
  vibetutor
```

Через docker-compose:

```bash
cp .env.example .env   # заполните токен и ключ
docker compose up -d --build
```

Данные сохраняются в каталоге `./data` на хосте.

## Конфигурация

Файл — обычный текст `ключ = значение`. Строки, начинающиеся с `#`,
игнорируются. Любое значение переопределяется переменной окружения с
префиксом `VIBETUTOR_`.

| Ключ в файле       | Переменная окружения              | Назначение                                   |
|--------------------|-----------------------------------|----------------------------------------------|
| `telegram_token`   | `VIBETUTOR_TELEGRAM_TOKEN`        | Токен бота от @BotFather (обязателен)        |
| `allowed_users`    | `VIBETUTOR_ALLOWED_USERS`         | ID пользователей через запятую (пусто = все) |
| `openai_base_url`  | `VIBETUTOR_OPENAI_BASE_URL`       | Адрес API, например `http://localhost:11434/v1` |
| `openai_api_key`   | `VIBETUTOR_OPENAI_API_KEY`        | Ключ API (для локальных моделей может быть пуст) |
| `openai_model`     | `VIBETUTOR_OPENAI_MODEL`          | Название модели                              |
| `openai_timeout`   | `VIBETUTOR_OPENAI_TIMEOUT`        | Таймаут запроса, например `60s`              |
| `data_dir`         | `VIBETUTOR_DATA_DIR`              | Каталог с данными пользователей              |

## Пример с локальной моделью (Ollama)

```ini
telegram_token = 123456:ABC...
openai_base_url = http://localhost:11434/v1
openai_api_key =
openai_model = llama3.1
```

## Разработка

```bash
make test     # go test ./...
make race     # go test -race ./...
make vet      # go vet ./...
make fmt      # gofmt
```

### Структура

```
cmd/vibetutor        точка входа, разбор флагов, graceful shutdown
internal/config      загрузка текстового конфига и переменных окружения
internal/ai          клиент OpenAI-совместимого chat completions API
internal/store       файловое хранилище состояния каждого пользователя
internal/tutor       логика репетитора: setup, plan, lesson, save, chat
internal/bot         адаптер telebot.v4: команды, обработка сообщений
```

## Приватность

Данные учеников (профиль, план, прогресс, история переписки) хранятся
локально в каталоге `data/` в виде JSON и не передаются никуда, кроме
настроенного вами API модели. Каталог `data/`, `config.txt` и `.env`
исключены из git.
