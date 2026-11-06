# redvanov_coach_bot

Telegram-бот на Go: `/start` доступен только владельцу в личном чате. Для локального запуска нужен Docker.

## Настройки

Заполни `.env` в корне проекта своим числовым Telegram ID и токеном от BotFather. Значения указывай без кавычек:

```dotenv
TELEGRAM_OWNER_ID=123456789
TELEGRAM_BOT_TOKEN=replace-with-BotFather-token
```

После клонирования создай `.env` командой `cp .env.example .env`. Локальный `.env` исключён из Git и Docker-образа; Docker получает настройки при запуске контейнера.

Перед деплоем добавь эти два значения в Environment сервиса Render. Без них приложение не запускается.

При старте проверяются наличие токена и корректность ID. Ошибки связи с Telegram и неверного токена выводятся в логи во время polling и не останавливают HTTP-сервер.

## Сборка и запуск

```sh
docker build -t redvanov-coach-bot .
docker run --rm --env-file .env -p 127.0.0.1:8080:8080 redvanov-coach-bot
```

В другом терминале:

```sh
curl --fail http://127.0.0.1:8080/
```

Ожидаемый ответ: `200 OK` с текстом `ok`. Это проверка, что HTTP-сервер отвечает; доступность Telegram она не проверяет. Остановить контейнер — `Ctrl+C`.

Для другого порта после остановки контейнера:

```sh
docker run --rm --env-file .env -e PORT=10000 -p 127.0.0.1:10000:10000 redvanov-coach-bot
```

Проверка в другом терминале: `curl --fail http://127.0.0.1:10000/`.

CI на PR и push в `main`: Go-тесты, сборка образа и HTTP-проверка контейнера.

## Подключение Telegram

Бот получает обновления через long polling. На один токен должен работать один экземпляр бота: не запускай локальную копию одновременно с Render.

Если webhook уже был установлен, удали его один раз перед запуском polling. В URL команды замени `TOKEN` значением `TELEGRAM_BOT_TOKEN` из `.env`:

```sh
curl --fail --silent --show-error --request POST "https://api.telegram.org/botTOKEN/deleteWebhook"
```

Команда сохраняет ожидающие обновления. Публичный адрес для Telegram не нужен; HTTP-сервер остаётся для проверки состояния Render. Реальные токены не добавляй в репозиторий.
