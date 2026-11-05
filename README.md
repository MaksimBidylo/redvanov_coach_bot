# redvanov_coach_bot

HTTP Hello World на Go. Для локального запуска нужен Docker.

## Сборка и запуск

```sh
docker build -t redvanov-coach-bot .
docker run --rm -p 127.0.0.1:8080:8080 redvanov-coach-bot
```

В другом терминале:

```sh
curl --fail http://127.0.0.1:8080/
```

Ожидаемый ответ: `Hello, world!`. Остановить контейнер — `Ctrl+C`.

Для другого порта после остановки контейнера:

```sh
docker run --rm -e PORT=10000 -p 127.0.0.1:10000:10000 redvanov-coach-bot
```

Проверка в другом терминале: `curl --fail http://127.0.0.1:10000/`.

CI на PR и push в `main`: Go-тесты, сборка образа и HTTP-проверка контейнера.
