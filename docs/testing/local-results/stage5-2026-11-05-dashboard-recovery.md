# Фактические локальные проверки: дашборд и recovery

Дата: 2026-11-05. **DASHBOARD_RECOVERY_LOCAL_PASS** только для указанного блока. VPS/VPN/systemd/ACME: **NOT RUN**, этапы 4–5 не ACCEPTED. Полный объём: [релиз](../../release-0.7.0-stage5-dashboard-recovery.md).

## Среда и команды

Linux arm64 Docker image `arcvpn-local-test:20261105`: Go 1.26.6, SQLite/CGO; frontend Node 22.21.1, TypeScript/Vite из lockfile. Команды Go выполнялись внутри контейнера с исходниками read-only и отдельным GOCACHE.

```sh
go test ./...
go vet ./...
go test -race ./cmd/arcvpn ./internal/store ./internal/httpapi ./internal/nodeagent ./internal/metrics
npm --prefix frontend test
npm --prefix frontend run build
```

Все — exit 0. Race: cmd 9.410s, store 1.512s, httpapi 85.511s, nodeagent 1.387s, metrics 1.037s. После финального уточнения allowlist CLI parser повторный `go test -race ./cmd/arcvpn` — PASS 6.856s. Дополнительный тест `TestLocalRecoveryRevokesLiveSessionAcrossDatabaseConnections` — PASS с race 1.035s. Frontend: **10 tests PASS**, TypeScript + production Vite build PASS (135 modules). Финальный Linux binary со встроенным актуальным React: `frontend-check` → `embedded React frontend: OK`.

Recovery tests проверяют сохранение активной MFA/удаление pending enrollment, явный reset MFA, отзыв сессий через отдельное подключение SQLite без рестарта HTTP, неизменность VPN credentials/subscription tokens, откат транзакции при отказе аудита, отсутствие паролей/seed/token в safe settings и аудите. Проверены root-before-open guard, stdin bounds/trailing injection, запрет password argv, allowlist runtime flags и units wrapper. Дополнительно в одноразовом Linux fixture созданы пользователь arcvpn/БД: реальные settings, stdin password reset, sessions revoke и explicit reset2fa через wrapper — PASS. Systemd в этой проверке заменён контролируемыми тестовыми командами.

Первый тест wrapper при чтении живого workspace получил shell EOF; fixture переведён на стабильную временную копию скрипта, повтор/full/race PASS. В первом UI прогоне обнаружен неоднозначный label select; исправлен явным aria-label, повтор PASS. Ошибки не замаскированы принятым PASS до повторной проверки.

## Chromium на настоящем API

Одноразовый backend `127.0.0.1:18181`, настоящее SQLite/auth/CSRF, без перехвата API. Playwright из отдельного `/private/tmp/arcvpn-browser/node_modules`; локальный Chromium 1228. Для повторения требуются одноразовый backend и приватный файл пароля, переменные `ARCVPN_E2E_URL`, `ARCVPN_E2E_PASSWORD_FILE`, `ARCVPN_CHROMIUM`; `NODE_PATH` указывает на установленный Playwright. Пароль/ключи в отчёт не включаются.

```sh
node tests/dashboard_e2e.mjs
node tests/traffic_profiles_e2e.mjs
node tests/routing_e2e.mjs
```

Все — exit 0. Dashboard: 23 синтетически зарегистрированные ноды + local; fresh/legacy/offline, первые 20/раскрытие24, поиск, no-store, отсутствие выдуманного VPN PASS. История дождалась реальных минутных bucket и двух delta, без backdated receipt/ручной инъекции DB. Проверены график, pointer/keyboard, обе темы и ширины 320/390/768/1024/1440; JS errors/горизонтальный overflow отсутствуют. Скриншоты `tests/screenshots/dashboard/` (не часть source archive), light390/dark1440 просмотрены визуально.

Регрессия: traffic profiles CRUD/import/export/off/CAS/draft retention и SNI SSRF reject; routing pointer/reserve/cycle/CAS/node dependencies/encrypted backup v5 preview; 8 страниц × 5 ширин × 2 темы. Всё PASS. Это проверка UI/control plane, не трафика.

## Ограничение ресурсов и измерения

Backend контейнер запущен с `--cpus=1 --memory=512m --pids-limit=64`, `GOMAXPROCS=1`. Inspect: NanoCpus=1000000000, Memory=536870912, OOMKilled=false, RestartCount=0. После всех браузерных сценариев: process VmHWM/VmRSS **23396 KiB**, threads7; docker memory **25.55 MiB / 512 MiB**, PIDS8. Контейнер включает socat; однократный idle CPU 0.00% не является нагрузочным benchmark. Здесь нет Xray/VPN-трафика или настоящих удалённых агентов. Procfs в Docker может отражать VM, а не cgroup budget.

Отдельные microbenchmarks в Linux arm64, Go1.26.6, не в ограниченном UI контейнере:

```sh
go test ./internal/metrics -run '^$' -bench BenchmarkSampler -benchmem
go test ./internal/store -run '^$' -bench BenchmarkOverviewUserCounts -benchmem
```

| Проверка | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| Sampler cached | 21.63 | 0 | 0 |
| Sampler collected fixture | 2676 | 10816 | 62 |
| Sampler Linux procfs | 53060 | 32728 | 177 |
| User count aggregate, 10000 rows | 381847 | 392 | 9 |
| Previous full user list, 10000 rows | 12021960 | 5190020 | 140021 |

Сравнение user count — отдельный warm SQLite query, не latency всей страницы. Оно подтверждает устранение чтения credentials/лишних allocations, не throughput полного VPN. Ни minimum RAM, ни max users, ни обещание отсутствия всех багов не устанавливаются.

## Остаток

Настоящие VPS/systemd/SSH/runtime, старые и новые агенты в реальной fleet, handshake/egress/отказы, cgroup-aware metrics, долговременные VPN нагрузки и полный restore всё ещё требуют внешней приёмки. Geo/runtime маршрутизации и client failover этим блоком не реализованы. Публикация маршрутов остаётся запрещена без ACK всех нод и достоверной независимой egress-пробы.
