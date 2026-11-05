# Hysteria2: учёт и устройства — 2026-11-05

Контракт/границы: [ADR-010](../../adr/010-hysteria-traffic-and-devices.md). Не является VPS/systemd/ACME приёмкой.

## Снимок и выполненные проверки

Source tar SHA256 `f0fe9872b3e1cc5d3778835f22c948fbe5578e656107caf716d2a8bfd7540925`. Linux arm64, существующий `golang:1.26` + подготовленные SQLite/qrencode; Node 22.16.0, прежний npm lockfile. Новых dependencies проекта нет, Python не использовался. Начальная macOS попытка упёрлась в недоступный Go cache; результаты ниже относятся к успешному Linux-прогону.

На неизменяемом снимке прошли, exit 0:

```sh
go test ./...
go vet ./...
go test -race ./internal/usage ./internal/protocols ./internal/engines ./internal/store ./internal/app ./internal/httpapi ./internal/backup
go build -o /output/hysteria-arcvpn ./cmd/arcvpn
ARCVPN_TEST_HYSTERIA=/output/hysteria-v2.13.0 go test -v ./internal/engines -run '^TestRealHysteriaAccountingAndRevokedDevice$' -count=1
```

Новые regression покрывают source freshness (Xray не делает отсутствующий Hysteria свежим), суммирование/общую квоту/replay, source masquerade denial, credentials устройств/отсутствие common/retired allowlist, private files, реального владельца loopback listener и epoch parser с микросекундами. HTTP API проверяет Hysteria в индивидуальной подписке и отказ экспорта/подписки при исчерпании общего пакета. Backup v8 переносит checkpoint, старый формат не принимает новый stream.

Финальный snapshot SHA256 `aae64d7316f743b958d95ae55b26d66032a9ca5911d3204df82aed238f8c0bf3` содержит уточнение version guard Hysteria device capability + regression; остальные Go inputs идентичны первому снимку. На нём прошли `go test ./internal/backup ./internal/app ./internal/httpapi`, `go vet ./internal/backup`, `go test -race ./internal/backup -run TestV8HysteriaCheckpointsRoundTripAndLegacyRejection -count=1` и итоговый `go build`, exit 0. Бинарник подтвердил version v2.13.0, commit ca8fbd874fd6ca82413948de6dc5924405e103e1. После проверки менялась только документация.

## Реальный QUIC/TLS

Официальный asset Hysteria **app/v2.13.0 linux-arm64** проверен до исполнения: SHA256 `a68a61a84452ca250ce0368202521965ca9cc9d801a404f1dc9008ac6cf677a7` совпал с GitHub release digest. Настоящие server+client, loopback-only UDP/SOCKS5/HTTP fixture, доверенный временный CA и SNI verification; insecure TLS не включался. Только в тестовой конфигурации пути сертификатов заменены на temporary paths, production validator остаётся allowlisted.

Передано 128 KiB тела, измерено **upload=115 / download=131230 bytes** (включают proxy HTTP headers). Повторный GET не сбросил counters. После настоящего restart с revoked allowlist старый device credential не подключился, независимый разрешённый credential передал то же тело. На финальном снимке real test PASS, 0.55 s. Это проверяет daemon/config/auth/counters/proc socket ownership; systemd MainPID/InvocationID boundary не выполнялся на живом systemd, fixture передаёт известный PID напрямую.

## Frontend/browser

`npm run build` — TypeScript/Vite PASS, 140 modules; `npm test` — 11 PASS, Node 22.16.0. Менялись только тексты существующих компонентов, новых UI abstractions/dependencies нет.

Opt-in `TestDevicesBrowserFixture` с `ARCVPN_DEVICE_BROWSER=1 ARCVPN_DEVICE_BROWSER_HYSTERIA=1` и `tests/devices_e2e.mjs` с `ARCVPN_E2E_HYSTERIA=1`: **PASS** на настоящем disposable auth/CSRF/API/SQLite, без перехвата API. Hysteria-профиль synthetic, Xray отсутствует, поэтому pending не выдаётся за исполнение. Проверены Hysteria URI отдельного устройства, общий ключ/старый subscription denial, limit/API bypass, отзыв/slot reuse, CAS с сохранением draft, pending apply и 5 widths × 2 themes (320/390/768/1024/1440), без JS errors/overflow. Stand завершился, 4.93 s; screenshots в `/private/tmp/arcnet-usage-check/hysteria-source/tests/screenshots/devices/`.

## Открытые gates

VPS/public QUIC reachability, реальный systemd/root helper/procfs isolation, IP ACME, перезагрузка/clock recovery, overshoot и production throughput не подтверждены. WG/AWG/remote devices/accounting и runtime каскадов не входят в этот блок. Внешних сообщений и production deploy не было.
