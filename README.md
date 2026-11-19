# TripGo — репозиторий для лабораторных работ

Заготовка курса «Разработка микросервисов на Go». Здесь вы делаете все пять
работ: каждая следующая продолжает предыдущую, переписывать сервис с нуля не
нужно.

## Trip Service: лабораторная работа 1

### Запуск

Понадобятся Go, `make` и установленный `tripgoctl`.
Локальное окружение поднимает PostgreSQL и создаёт `.env` с актуальным адресом:

```bash
tripgoctl cluster start
tripgoctl environment start

set -a
source .env
set +a

make migrate
make run
```

Перед запуском должны быть заданы все переменные из `.env.example`, включая таймауты HTTP сервера, которые нужно задать вручную в `.env`.  
  
Основные команды:
```bash
make generate  # повторно сгенерировать Go-типы и chi-сервер из OpenAPI
make migrate   # применить миграции к DATABASE_URL
make test      # выполнить все тесты
make lint      # golangci-lint (с лабораторной работы 2)
make run       # запустить trip-service
```

### Переменные окружения

| Переменная | Назначение |
|---|---|
| `HTTP_ADDR` | адрес HTTP-сервера |
| `HTTP_READ_TIMEOUT` | таймаут чтения запроса |
| `HTTP_READ_HEADER_TIMEOUT` | таймаут чтения заголовков |
| `HTTP_WRITE_TIMEOUT` | таймаут записи ответа |
| `HTTP_IDLE_TIMEOUT` | таймаут keep-alive соединения |
| `DATABASE_URL` | строка подключения PostgreSQL |
| `DATABASE_MAX_CONNS` | максимальный размер пула |
| `DATABASE_MIN_CONNS` | минимальный размер пула |
| `DATABASE_MAX_CONN_LIFETIME` | максимальное время жизни соединения |
| `DATABASE_CONNECT_TIMEOUT` | таймаут подключения к PostgreSQL |
| `DATABASE_QUERY_TIMEOUT` | таймаут запросов и транзакций |
| `SHUTDOWN_TIMEOUT` | таймаут graceful shutdown |
| `LOG_LEVEL` | минимальный уровень JSON-логов (`debug`, `info`, `warn`, `error`) |

### Решения

Транзакции выполняются с уровнем изоляции `READ COMMITTED`. Для этой работы его
достаточно: конкурентные инварианты обеспечиваются атомарным условным `UPDATE`
и ограничениями PostgreSQL, а более строгая изоляция добавила бы лишние ошибки
сериализации без необходимости.

Менеджер транзакций открывает транзакцию, сохраняет `pgx.Tx` в `context.Context`
и передаёт обогащённый контекст callback-функции. Репозиторий выбирает
исполнителя из контекста: внутри `Do` используется транзакция, в остальных случаях — пул.
Вложенный `Do` обнаруживает существующую транзакцию и переиспользует её.
Успешный callback приводит к `COMMIT`, ошибка или паника — к `ROLLBACK`.
Операции транзакции ограничены `DATABASE_QUERY_TIMEOUT`.

Создание поездки и начальная запись в `trip_status_history` выполняются в одной
транзакции. Запрет двух активных поездок одного водителя закреплён частичным
уникальным индексом по `driver_id WHERE status = 'active'`; нарушение индекса
распознаётся по PostgreSQL-коду `23505` и возвращается как `409 driver_busy`.

Завершение поездки использует один условный `UPDATE` с `status = 'active'`.
Поэтому два конкурентных запроса не могут оба завершить поездку: один обновляет
строку, второй получает ноль строк и возвращает `409 trip_completed`.

## Содержимое курса

| Что | Где |
|---|---|
| Задания, документация, контракты | [`course-go-autumn-2026/course`](https://github.com/course-go-autumn-2026/course) |
| Слайды и записи лекций | [`lections/`](https://github.com/course-go-autumn-2026/course/tree/main/lections) |
| Как оценивают, дедлайны, порядок сдачи | [`homework/docs/grading.md`](https://github.com/course-go-autumn-2026/course/blob/main/homework/docs/grading.md) |
| Локальное окружение и утилита `tripgoctl` | [`course-go-autumn-2026/course-infra`](https://github.com/course-go-autumn-2026/course-infra) |
