# WireGuard: source/API/UI — 2026-11-06

[Контракт](../../adr/011-wireguard-traffic-and-devices.md). AmneziaWG не расширяется по прямому решению пользователя. Это не kernel/VM/VPS acceptance.

## Исходники и среда

Неизменяемый source tar SHA256: `ef0ceaa59fe411f9e0fea9037766abdd28786e4e9fc8a018a4669e9900d02dfb`. Linux arm64 `golang:1.26`, прежние SQLite/qrencode libraries; Node 22.16.0 и существующий npm lockfile. Без новых dependencies, установки WG tools и Python. После снимка менялась только документация.

Итоговый pipeline:

```sh
go test ./...
go vet ./...
go test -race ./internal/devices ./internal/protocols ./internal/engines ./internal/store ./internal/app ./internal/httpapi ./internal/backup
go build -o /output/wireguard-arcvpn ./cmd/arcvpn
```

Все четыре команды завершились с exit 0 / PASS. HTTP race — 101.433 s, backup race — 19.021 s. На момент commit Go/frontend исходники и тесты идентичны снимку; после проверок менялась только документация.

Покрыты генерация/валидация WG device keypair, отсутствие общего и отозванного peer в server config, ClientTunnel с индивидуальным IP, private keys не попадают в публичный stats report, parse public-key/rx/tx/duplicate/missing-peer/revision/server-key checks, fixed RPC transport на root-owned Unix fixture. Store regression подтверждает адрес без коллизии с legacy credential, legacy restore без ротации локальных ключей, выбор другого IP при новой коллизии, общий расход/replay/independent WG freshness/quota и needs_review после неожиданной смены epoch. Backup v9 сохраняет ключи/IP/checkpoints, новая capability под v8 запрещена. API проверяет no-store `.conf`, отсутствие secrets в списке, expiry и revoke denial.

Во время реализации исходный negative test был обновлён: WireGuard теперь поддержан, запрещённым остаётся AmneziaWG. Тест адресов сначала неверно предполагал порядок ListUsers; исправлен lookup по subscription identity, проверка отсутствия коллизии сохранена. Последующий targeted Store/API/App запуск PASS; assertions не ослаблялись.

## UI

`npm run build` (TypeScript/Vite, 140 modules) и `npm test` (11 tests) — PASS. Browser: opt-in `TestDevicesBrowserFixture` с `ARCVPN_DEVICE_BROWSER=1 ARCVPN_DEVICE_BROWSER_WIREGUARD=1`; `tests/devices_e2e.mjs` с `ARCVPN_E2E_WIREGUARD=1`. Настоящие disposable HTTP/auth/CSRF/SQLite, без подмены API; VPN runtime намеренно недоступен и показан pending.

Browser **PASS**, stand 6.45 s: автоматический IP, ссылка скачивания WireGuard/содержимое `.conf`, отказ старому URL после revoke, limit/API bypass, slot reuse, CAS с сохранением введённого draft, общий ключ отключён, пять размеров 320/390/768/1024/1440 и обе темы, без JS errors/overflow. Финальный 390px screenshot просмотрен визуально: IP и кнопка WireGuard в существующих design-system компонентах. Screenshots: `/private/tmp/arcnet-usage-check/wireguard-source/tests/screenshots/devices/`.

## NOT RUN / внешняя приёмка

Новый root epoch stamp/cleanup и read-only stats **не выполнялись на настоящем WG интерфейсе**. Нет живых `wg-quick up/down`, kernel peer traffic/handshake, systemd integration или isolated VM rollback при ошибке stamp/потере процесса. Go root-owned socket fixture — не proof этих свойств. Не проверены public VPS/ACME/egress, byte overshoot или throughput. До этих испытаний runtime/enforcement EXTERNAL_PENDING; прежний трафик при failed apply может продолжаться.
