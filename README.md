# TripGo — репозиторий для лабораторных работ

Заготовка курса «Разработка микросервисов на Go». Здесь вы делаете все пять
работ: каждая следующая продолжает предыдущую, переписывать сервис с нуля не
нужно.

## Trip Service

### Запуск

Понадобятся Go, `make` и установленный [`tripgoctl`](https://github.com/course-go-autumn-2026/course-infra).
Локальное окружение поднимает PostgreSQL и коллектор телеметрии (Jaeger,
Prometheus, Grafana) и создаёт `.env` с актуальными адресами:

```bash
tripgoctl cluster start
tripgoctl environment start
tripgoctl connect        # ссылки на Jaeger, Grafana, Prometheus

make migrate
make run
```

`Makefile` подхватывает `.env` сам. `tripgoctl` не пишет в `.env` таймауты
HTTP-сервера (`HTTP_*_TIMEOUT`), их нужно добавить вручную, значения есть в
`.env.example`. Без любой обязательной переменной сервис не стартует и пишет,
какой не хватает.

```bash
make generate          # код по OpenAPI и моки (go generate)
make migrate           # миграции в DATABASE_URL
make run               # запустить trip-service
make lint              # golangci-lint, конфиг в .golangci.yml
make fmt               # gofumpt + gci
make test              # unit-тесты с -race, печатает покрытие domain + usecase
make test-integration  # тесты репозитория на PostgreSQL из tripgoctl
```

### Тесты

- `make test` гоняет все unit-тесты с `-race` и печатает итоговое покрытие
  пакетов `internal/domain/...` и `internal/usecase/...`. Внешние зависимости
  в них только моки `gomock`, сгенерированные `go:generate` в `mocks_test.go`.
- `make test-integration` запускает тесты с билд-тегом `integration` против
  базы из `tripgoctl environment start` (адрес берётся из `DATABASE_URL`).
  Каждый тест создаёт поездки со своими ID и удаляет их после себя, поэтому
  тесты независимы и проходят повторно (`-count=2`). Среди них тест на откат:
  `INSERT` в `trips` проходит, `INSERT` в `trip_status_history` отклоняет
  `CHECK`-ограничение базы, и после `ROLLBACK` нет ни одной из двух строк.

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
| `OTEL_SERVICE_NAME` | `service.name` в трейсах и метриках |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | адрес коллектора |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | только `http/protobuf` |
| `OTEL_RESOURCE_ATTRIBUTES` | `service.namespace`, `deployment.environment` |
| `OTEL_TRACES_SAMPLER` | сэмплер трейсов, например `parentbased_always_on` |

### Архитектура

Зависимости направлены внутрь: транспорт и репозиторий знают про usecase,
usecase знает про домен, домен не знает ни про кого.

| Слой | Пакеты | Что делает | Что не импортирует |
|---|---|---|---|
| домен | `domain/trip` | `Trip`, `New` с валидацией, `Finish`, доменные ошибки | ничего кроме stdlib и `uuid` |
| usecase | `usecase/trip/{create,get,finish}` | один сценарий на пакет, границы транзакции, бизнес-логи и счётчики | `net/http`, `chi`, `pgx`, `squirrel`, `generated` |
| транспорт | `http/trip/{create,get,finish}`, `http/health`, `http/domainerror`, `http/middleware`, `http/router` | разбор запроса, ответ, маппинг ошибок, middleware | `repository`, `infra` |
| данные | `repository/trip` | SQL через squirrel, перевод ошибок PostgreSQL в доменные | транспорт |
| инфраструктура | `infra/postgres`, `infra/telemetry` | пул, транзакции, OpenTelemetry, логгер | — |

Правила проверяет `depguard` в `.golangci.yml`, поэтому нарушение слоя
ломает `make lint`.

Все зависимости создаются и связываются в `internal/container`: это
единственное место, где встречаются `pgxpool.New` и конструкторы
репозиториев. `cmd/trip-service/main.go` читает конфигурацию, вызывает
`container.New`, запускает сервер и ждёт сигнала остановки.

**Где объявлены интерфейсы и почему.** Интерфейсы объявляет потребитель, в
`contract.go` своего пакета, и они неэкспортируемые: `usecase/trip/finish`
объявляет `tripRepository` ровно с теми тремя методами, которые вызывает, и
`txManager` с одним `Do`. Реализация (`repository/trip.Repository`) ничего про
эти интерфейсы не знает, в Go они удовлетворяются неявно. Так usecase
формулирует, что ему нужно, интерфейсы остаются узкими, моки генерируются по
одному файлу, а реализацию можно заменить, не трогая usecase. Тем же правилом
хендлеры объявляют интерфейс своего usecase, а usecase объявляет
`tripCounter` вместо зависимости от OpenTelemetry.

**Отступления от раскладки задания.** Общий пакет `internal/model` не
понадобился: вход сценария создания, `create.Input`, нужен только usecase
`create` и его хендлеру, поэтому лежит в самом usecase. Поездку между слоями
передаёт доменный `trip.Trip`. Ответы и ошибки HTTP пишет пакет
`http/respond`, чтобы не дублировать это в каждой ручке.

### Observability

- **Трейсы.** Middleware `Tracing` (`otelhttp`) создаёт корневой спан
  `{method} {http.route}`, например `POST /api/v1/trips`. Каждый запрос в базу
  даёт дочерний спан с `db.operation.name` и `db.collection.name`: трейсер
  повешен на пул pgx (`infra/postgres/tracer.go` поверх `otelpgx`), репозиторий
  про телеметрию не знает. Ответ 5xx и ошибка запроса в базу помечают спан
  `codes.Error` и `error.type`.
- **Метрики.** `http.server.request.duration`,
  `http.server.active_requests`, `db.client.operation.duration`,
  `tripgo.trips.created`, `tripgo.trips.completed`. В `http.route` всегда
  шаблон маршрута: с подставленным `tripId` каждая поездка заводила бы свою
  временную серию.
- **Логи.** `slog`, JSON в stdout. Обработчик в `infra/telemetry/log.go`
  добавляет в запись `trace_id` и `span_id`, если в контексте есть спан, так
  по `trace_id` из лога находится трейс в Jaeger. Логгер передаётся
  зависимостью.
- **Middleware**: `Recover → Tracing → Logging → Metrics → роутер`. `Logging`
  стоит внутри `Tracing`, поэтому его запись `request handled` уже видит спан
  запроса. Уровень `Info` для 2xx и 4xx, `Error` для 5xx, тела не логируются.
- Провайдеры OpenTelemetry передаются зависимостями, глобальные
  `otel.SetTracerProvider` не используются. Если коллектор недоступен,
  экспорт падает в фоне с предупреждением в логе, запросы обслуживаются как
  обычно.

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

Завершение поездки: usecase загружает поездку, доменный `Trip.Finish` проверяет
переход статуса, репозиторий сохраняет результат условным `UPDATE ... WHERE
status = 'active'`. Доменное правило даёт понятную ошибку, а условие в `UPDATE`
защищает от гонки: из двух одновременных запросов второй получает ноль строк и
`409 trip_completed`.

Ошибки домена объявлены в `domain/trip/errors.go`, ошибки валидации несут поле
и причину (`*trip.ValidationError`). Перевод в HTTP собран в одном месте,
`http/domainerror.Problem`. Любая неизвестная ошибка становится `500
internal_error` с общим текстом, сама ошибка пишется в лог один раз. Ошибка
`ErrFinishBeforeStart` тоже даёт 500: время завершения выставляет сервер, и
клиент не может её вызвать.

Структуры передаются между слоями по значению, указатель возвращается там, где
результата может не быть (`*trip.Trip, error`). Метод `Trip.Finish` меняет
поездку и поэтому определён на указателе.

## Содержимое курса

| Что | Где |
|---|---|
| Задания, документация, контракты | [`course-go-autumn-2026/course`](https://github.com/course-go-autumn-2026/course) |
| Слайды и записи лекций | [`lections/`](https://github.com/course-go-autumn-2026/course/tree/main/lections) |
| Как оценивают, дедлайны, порядок сдачи | [`homework/docs/grading.md`](https://github.com/course-go-autumn-2026/course/blob/main/homework/docs/grading.md) |
| Локальное окружение и утилита `tripgoctl` | [`course-go-autumn-2026/course-infra`](https://github.com/course-go-autumn-2026/course-infra) |
