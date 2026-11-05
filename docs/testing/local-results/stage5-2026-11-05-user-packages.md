# Проверки локальных пакетов Xray — 2026-11-05

Проверен локальный контур, **не VPS/REALITY/systemd quota enforcement**. [Контракт и ограничения](../../adr/008-local-xray-traffic-packages.md).

## Привязка к исходникам

Go/source snapshot manifest SHA256: `c0db9cc23bbe3ccd4a27540719b9b2fc663ee208fcf9458c6f1ce7790300cca1`. После Go-проверок менялись только PackageDialog и его browser test; финальный delivery manifest: `4b203052a2c92a02620ca5f644a5dea08c1b6afb7838b6e58e1f248f14e740d6`. Go-исходники и тесты идентичны первому снимку. Проверенный Go-бинарник обслуживал финальную frontend-сборку. Документы актуализированы после проверок.

## Фактически выполнено

Linux arm64 Docker, Go 1.26.3, native executable tmpfs, существующие SQLite/qrencode libraries. Node 22.16.0, зависимости из прежнего lockfile, без новых зависимостей.

```sh
go test ./...
go vet ./...
go test -race ./internal/usage ./internal/store ./internal/app ./internal/httpapi ./internal/xray
go build -o /output/verified-arcvpn ./cmd/arcvpn
npm test
npm run build
bash -n tests/run-user-usage-browser.sh
git diff --check
```

Все — exit 0. Race: usage 1.012s, store 1.179s, app 3.374s, httpapi 100.079s, xray 1.022s. Frontend: **11 tests PASS**, TypeScript/Vite production build PASS, 139 modules. После финальных изменений формы повторён её build и затронутый browser E2E; неизменённые Go checks переиспользованы.

Покрыты replay/регрессия/смена epoch, атомарность debit failure, CAS/reset без повторного списания, known/unknown, controlled/unexpected restart, сохранение credentials, apply retry, откат удаления с accounting, закрытие экспорта при истечении срока, auth/CSRF, finite-source gate, legacy/high-water restore и encrypted v6. Round-trip **65536 debit rows**: архив **6991706 байт**, PASS.

## Настоящий Xray и браузер

```sh
ARCVPN_TEST_XRAY=/output/xray go test -v ./internal/xray -run TestRealXrayStatistics -count=1
node tests/user_usage_e2e.mjs
node tests/routing_e2e.mjs
```

Все — exit 0. По умолчанию реальный Xray integration test делает SKIP; указанная отдельная команда действительно выполнена на frozen Go source. Официальный тестовый Xray **v26.6.1 Linux arm64**, asset SHA256 `5c0f397b2605a853e5b308ae6485ad2ec395ca11ed7ae0fd023c480c7fdb279d`. Loopback VLESS/SOCKS передал HTTP body 128 КиБ; StatsService показал **92 upload / 131669 download**, повторный запрос не сбросил counters. Разрешение loopback добавлено только в тестовую конфигурацию, не production renderer. Это не REALITY или внешний VPN handshake.

Chromium 156/Playwright на macOS: реальный disposable API/SQLite/auth/CSRF, синтетические сохранённые user/config, Xray намеренно недоступен. Проверены отсутствие ложного нуля, запрет конечного пакета без источника, CAS с сохранением полей/явным принятием новой версии, pending apply, срок/отказ выдачи ссылки, plan diff, пять размеров 320/390/768/1024/1440 и обе темы, без JS errors/overflow. После визуальной проверки использованы существующие dialog-body/flex отступы, повтор E2E PASS. Shell wrapper проверен синтаксически; browser-фазы выполнены через Linux backend/macOS Chromium, не wrapper целиком.

Routing regression: pointer/резервы/циклы/CAS, зависимости нод, encrypted **v6** preview, **8 страниц × 5 размеров × 2 темы**, PASS. После него менялась только открытая форма пакета; её новый E2E повторён отдельно.

## Измерение горячего пути

`go test ./internal/store -run '^$' -bench BenchmarkUserAccess10000Users -benchmem -benchtime=200ms -count=1`, одна и та же Linux-среда и 10000 fixture users:

| Проверка | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| Первоначальное чтение всех пакетов | 11544456 | 6356462 | 80074 |
| Атомарный point lookup | 9434 | 552 | 16 |

Это warm SQLite access check, не производительность VPN, minimum RAM или max users.

## Ограничения

При первоначальном запуске real fixture Xray штатно блокировал loopback; исправлена только fixture. На macOS bind mount не поддерживался chmod socket; тесты перенесены на native private tmpfs без ослабления production-проверок. Доступные имена полей и отступы формы исправлены после browser/visual QA. Последние прогоны после этих исправлений прошли.

Устройства, UDP/remote accounting, календарные периоды, каскады и реальный disconnect/overshoot не проверены и не приняты. Контролируемый restart может потерять неснятый хвост — UI отмечает неполноту; неожиданный restart закрывает конечный пакет до решения оператора. При отказе root helper ранее работающий VPN может продолжаться, статус apply_failed не означает блокировку.
