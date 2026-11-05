# Stage5 draft routing UI — фактические проверки 2026-11-05

## Среда

- Host: macOS arm64, Node 22.21.1; npm frontend build/typecheck.
- Linux: одноразовый Docker-контейнер Debian bookworm arm64, Go 1.26.6, libsqlite3-dev 3.40.1, libqrencode4 4.1.1. Без systemd, SSH или настоящего Xray.
- Браузер: Playwright 1.58.2, реальный локальный Chromium 149.0.7827.55; production Vite bundle встроен в Go через `arcvpn_embed_react`. Go backend работает на временной SQLite. Loopback HTTP проксируется только внутри временного контейнера; на хосте доступен через 127.0.0.1. TLS-проверки приложения не отключались.

## Команды и результаты

| Проверка | Фактический результат |
|---|---|
| `npm ci --no-audit --no-fund` с реальным `package-lock.json` | PASS, зависимости установлены |
| `npm run test` | PASS, 5 тестов переходов формы, связей, циклов, порядка и валидации |
| `npm run typecheck` | PASS |
| `npm run build` | PASS, Vite 7.3.7, настоящий production bundle |
| `npm audit --json` | PASS, 0 найденных уязвимостей на момент проверки; не гарантия отсутствия всех уязвимостей |
| Linux `bash scripts/verify-stage5-source.sh` | PASS: все Go tests, vet, routes/store race, CLI build |
| Linux `go test -count=1 ./... && go vet ./...` | PASS после изменений backup и HTTP |
| Linux `go test -count=1 -race ./internal/routes ./internal/store ./internal/backup ./internal/app ./internal/httpapi` | PASS, включая HTTP race (76.119 s) |
| Linux `go build -tags arcvpn_embed_react -buildvcs=false -o /test/arcvpn-final ./cmd/arcvpn` | PASS, тестовый Linux arm64 binary, не signed release |
| `bash -n tests/run-routing-browser.sh scripts/verify-stage5-source.sh`; `node --check tests/routing_e2e.mjs` | PASS |
| `tests/routing_e2e.mjs` против настоящего Go API | PASS, настоящие основная/резервная стрелки мышью и 80 комбинаций страниц/размеров/тем, без JS errors |

Backend regression coverage: зависимости входа/основного/резервного выхода, гонка route save против SSH removal, отказ legacy import с orphan refs без изменения нод/лог-бюджета, атомарная замена/очистка v4, увеличение CAS, шифрованный roundtrip и чтение v1/v2/v3, malformed graph/revision, rollback маршрутов при отказе движка, auth/CSRF/HTTP 409, непустой и пустой `backup_exits` как массив.

UI regression scenario: реальное соединение основной и резервной стрелок мышью; перестановка резервов; очистка полей при изменении входа/выхода; сброс формы; server preview; сохранение draft; cycle rejection; конфликт двух редакторов с сохранением локального ввода; блокировка удаления зависимой ноды; encrypted v4 export/preview; восемь страниц на 320/390/768/1024/1440 px в двух темах; проверка отсутствия горизонтального page overflow и JS errors. Снимки тестовой схемы сохраняются в `tests/screenshots/routing/` и не коммитятся.

## Воспроизведение браузерной проверки на Linux

Нужен собранный embedded binary с libsqlite3/libqrencode. Playwright устанавливается как отдельный инструмент тестирования, без runtime-зависимости панели:

```bash
npm install --prefix /tmp/arcvpn-browser-tools --no-audit --no-fund playwright@1.58.2
node /tmp/arcvpn-browser-tools/node_modules/playwright/cli.js install --with-deps chromium
NODE_PATH=/tmp/arcvpn-browser-tools/node_modules ARCVPN_BINARY=/absolute/path/to/embedded-arcvpn tests/run-routing-browser.sh
```

Runner сам создаёт временного администратора/базу, запускает backend на loopback, выполняет тест и удаляет стенд. Не направлять standalone `routing_e2e.mjs` на пользовательскую/production базу.

## Ограничения

Прежний `tests/react_e2e.py` не запускался в этом блоке; новая Node E2E-проверка — отдельный сценарий с настоящим UI/API, а не фиктивная отметка старого теста. Linux x86_64 release pipeline, сборка AWG upstream, systemd/SSH/UFW, две/три VPS, независимый VPN-клиент, egress и реальный failover не проверялись. На macOS общий Go script не является подходящим target: Linux syscall/QR linker и sandbox sockets ранее блокировали его; для полного прогона использован Linux.

Статус принятия этапов 4–5 не изменён: внешняя приёмка отсутствует. Данные UI — черновики, не подтверждённый VPN.
