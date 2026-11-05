# Go: целевая структура и правила пакетов

**Пути проектные.** Создавать директорию только одновременно с реальным кодом её владельца. Это адаптация переданного пользователем документа «Архитектура универсального Auth».

```text
cmd/
  vpnctl/                 # флаги, сигналы, запуск panel/controller/hybrid
  vpn-agent/              # опциональный отдельный минимальный запуск node
internal/
  app/                    # сборка и lifecycle, cleanup
  config/                 # load/validate startup config
  server/http/            # timeouts, graceful shutdown
  transport/http/         # роутинг, общие middleware, problem responses
  auth/                   # admin identity, session, totp
  users/                  # пользователи, квоты, группы, credentials
  connections/            # профили, inbounds/outbounds, транспорты
  nodes/                  # узлы, роли, ревизии, применение
  routes/                 # каскады и политики маршрутов
  subscriptions/          # выдача/публикация подписок
  security/               # системные политики безопасности
  telemetry/              # сбор, агрегация, health
  backups/                # экспорт, snapshot, restore
  audit/                  # события и их запись
  integrations/
    xray/                 # Xray API, конфиг-генератор
    ssh/                  # SSH bootstrap и known_hosts
    acme/                 # сертификаты IP
  infra/
    sqlite/               # DB connection, migrations executor
    systemd/              # lifecycle контролируемых служб
    firewall/             # адаптер UFW/nftables
web/                      # React + Vite + TypeScript
  src/
    app/                  # router, providers, boot
    shared/               # ui, api, lib, styles
    entities/             # user, server, connection, route
    features/             # user-edit, server-install, failover...
    widgets/              # summary, charts, sidebar
    pages/                # screens для React Router
migrations/
  sqlite/
deploy/
  systemd/
docs/
sandbox/
  tests/                  # сквозные проверки реальных сервисов
  fixtures/               # искусственные данные, без production secrets
Taskfile.yml
AGENTS.md
```

## Владение ответственностями

| Слой/папка | Что делает | Чего не делает |
|---|---|---|
| `domain` фичи | состояния, инварианты, допустимые переходы | HTTP, SQL, системные команды |
| `service` | use-case, права, orchestration, узкие порты | JSON/HTTP DTO, прямые shell, конкретные SQL adapters |
| `repository/sqlite` | транзакции, constraints, optimistic version, чтение/запись | решение бизнес-политики |
| `transport/http` | форма запроса, валидация транспортной формы, ответы | бизнес-решения и прямой доступ к базе |
| `integrations` | протокол внешнего инструмента (Xray, SSH, ACME) | назначение пользователям доступа |
| `app` | связывание реализаций, lifecycles | бизнес-правила |
| `infra` | общая техника (DB, firewall/systemd drivers) | продуктовые политики |

Для маленькой фичи допустимо всё в 2–3 файлах: нельзя создавать четыре слоя ради оформления. Если несколько независимых сценариев имеют свои зависимости, разбить фичу по смыслу.

## Конкурентность и транзакции

- Один владелец SQLite-транзакции на связанную локальную операцию. Не дробить её на независимые commits ради красоты пакетов.
- Внешний SSH/agent/Xray вызов не происходит внутри SQL-транзакции. Для продолжения после commit — durable operation/revision log, повторы идемпотентны.
- Явные таймауты, cancellable context, bounded workers/queues; запрещены «goroutine на любой запрос» и бесконечные retry.
- SQLite WAL не эквивалент распределённой БД; записи конкурируют, read snapshots и checkpoint должны быть контролируемы.
- Не создавать `pkg/` для внутреннего кода; public APIs экспортировать только реальным внешним потребителям.

## Тесты

Для развитой фичи public tests могут быть в локальном `tests/` подпакете, если они используют публичный контракт. Private tests — рядом с реализацией; не экспортировать внутренности ради теста. Интеграционный sandbox вызывает реальные Go API, Xray и UFW в изолированном окружении; не подменяет поведение приложения. Коды `go test ./...` и отдельного пакета покрывают разные наборы.

## Ориентир размера файлов

Смысловой ориентир ~150 строк рукописного Go, **не числовой запрет**. Не разрывать связную транзакцию или use-case ради количества строк. Исключены generated/tests.
