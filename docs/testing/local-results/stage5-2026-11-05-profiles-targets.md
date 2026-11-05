# Локальные проверки: профили и SNI target

Дата: 2026-11-05. Рабочий каталог `/Users/a.tatarchuk/Desktop/projects/arcvpn`. Статус `LOCAL_SOURCE_CHECKS_PASS`, **не** приёмка этапов 4/5 или VPN.

## Go/Linux

Образ `arcvpn-local-test:20261105`, Go 1.26.6 / Linux arm64 / SQLite CGO; проект смонтирован read-only, GOCACHE в отдельном `/tmp/go-cache`.

```
docker run --rm -v /Users/a.tatarchuk/Desktop/projects/arcvpn:/workspace:ro -v /private/tmp/arcvpn-linux-go-cache:/tmp/go-cache -w /workspace -e GOCACHE=/tmp/go-cache arcvpn-local-test:20261105 sh -c 'go test ./... && go vet ./... && go test -race ./internal/routingprofile ./internal/publicnet ./internal/targetcheck ./internal/store ./internal/httpapi ./internal/nodeagent'
```

Exit 0. Дополнительно после нового теста lease/capability: `go test -race ./internal/store` в том же контейнере; exit 0 (1.547s). Targeted host-проверки routingprofile/publicnet/store/backup прошли; host TLS test не запустился из-за запрета sandbox bind localhost (exit 1). Та же TLS-проверка фактически прошла в Linux-контейнере; host-ошибка не выдана за PASS.

Покрыто: строгий импорт/лимиты/канонизация, выключенный профиль и порядок, неизвестные DNS/Geo без ложного выхода; CAS и rollback DB/legacy preservation; encrypted backup v5; HTTPS/IP/SSRF/DNS mixed answers/rebinding pin; TLS trust/name/TLS1.3/ALPN/h2/HTTP redirect/status; expiry/signature/replay каталога; auth/CSRF/размер HTTP; agent wire version/node/expiry/digest и старый контроллер; пересчёт метрик и сброс поддельного VPN proof.

TLS fixtures — локальный HTTPS server с отдельным тестовым CA в памяти; production trust verification не отключена. Не проверялся настоящий target в интернете, REALITY/Xray handshake или сеть провайдера.

## Frontend

В `frontend`: `npm run typecheck`, `npm run build`, `npm test` — exit 0. Последний build включает tsc project build и Vite production assets; 7 frontend model tests PASS. Новых зависимостей нет.

`go build -tags arcvpn_embed_react -buildvcs=false -o /test/arcvpn-final ./cmd/arcvpn` — exit 0 в Linux; assets предварительно скопированы из `frontend/dist` в `web/react-dist`. `arcvpn-final frontend-check` — `embedded React frontend: OK`.

## Chromium с настоящим Go API

Chromium 149.0.7827.55 / Playwright 1.58.2, одноразовый backend в Docker, внешний bind только `127.0.0.1:18181`. Backend создан через `admin-init`, пароль передан runner из private файла без вывода в чат. Локальный HTTP разрешён только для тестовой loopback-связи; это не публичная установка. В финальном запуске Go отдавал production assets через `--web-root /test/ui`; те же assets включены в embedded build. Перехвата API и подменённых ответов нет.

```
NODE_PATH=/private/tmp/arcvpn-browser/node_modules ARCVPN_E2E_URL=http://127.0.0.1:18181 ARCVPN_E2E_PASSWORD_FILE=/private/tmp/arcvpn-ui-check/password ARCVPN_CHROMIUM='/Users/a.tatarchuk/Library/Caches/ms-playwright/chromium-1228/chrome-mac-arm64/Google Chrome for Testing.app/Contents/MacOS/Google Chrome for Testing' node tests/traffic_profiles_e2e.mjs
```

Exit 0: CRUD профиля, JSON import preview/export, файл списка/дедупликация, model check, выключение с сохранением списков, dirty draft после закрытия, CAS/сохранение локальных правок; modal 320/390/768/1024/1440 px × light/dark без горизонтального переполнения. UI отклонил `metadata.internal` через реальный API; каталог не изменился. Внешних SNI-соединений этот E2E не делает.

Тот же runner/env с `node tests/routing_e2e.mjs` — exit 0: pointer-связь каскада, порядок резервов, очистка конфликтующих полей, cycles/CAS/node dependency, encrypted v5 preview, 8 страниц × 5 ширин × 2 темы, без JavaScript errors.

Первый browser run выявил неустойчивое accessible name textarea — добавлено явное имя поля; следующий выявил дедупликацию raw vs canonical доменов — исправлена нормализацией обеих сторон. Оба сценария после исправления прошли. Скриншоты локально в `tests/screenshots/{traffic-profiles,routing}`, в source archive не входят.

## Что не проверено/не реализовано

- Тестовых VPS нет по ответу пользователя. ACME/install/systemd/SSH/UFW rollback, реальные VPN и независимый egress не запускались.
- Подбор target не подтверждает совместимость конкретного REALITY/Xray, доступность из сети устройства или неотслеживаемость. Внешний подписанный feed не настроен.
- Профили ещё не применяются в runtime/подписках; Geo-базы не скачиваются; JSON экспорта собственный ArcVPN, не подтверждённый Happ import.
- Клиентское автопереключение протокола/сервера — пока [контракт](../../features/client-auto-switching.md), не executable/client export. Серверный каскад остаётся draft-only.
