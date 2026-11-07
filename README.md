# redvanov_coach_bot

Telegram-бот тренера на Go.

## Настройка

```sh
cp .env.example .env
```

Заполни `TELEGRAM_OWNER_ID` и `TELEGRAM_BOT_TOKEN` в `.env`.
`DATABASE_URL` задаёт подключение бота к PostgreSQL.

## Запуск

```sh
docker compose up -d --build --wait
```

PostgreSQL: `127.0.0.1:5432`, имя базы, пользователь и пароль — в `.env`.

Остановка — `docker compose down`. Данные базы сохраняются.

Для одного токена запускай один экземпляр бота.
