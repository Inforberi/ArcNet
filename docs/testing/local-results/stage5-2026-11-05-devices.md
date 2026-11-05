# Устройства локального Xray — 2026-11-05

Контракт: [ADR-009](../../adr/009-device-credentials.md). Это source/UI/API проверки, не приёмка VPS, REALITY handshake или живого disconnect.

## Исходники и среда

Первый неизменяемый source tar SHA256: `c2614565a0824b85d41c1299471e0ab19666a36571a16d9f02498811ba7d5137`. Linux arm64 Docker `golang:1.26` с ранее подготовленными SQLite/qrencode libraries, Node 22.16.0 и существующий npm lockfile. Без новых зависимостей проекта и Python.

На первом снимке прошли (exit 0):

```sh
go test ./...
go vet ./...
go test -race ./internal/devices ./internal/store ./internal/app ./internal/httpapi ./internal/xray
```

Покрыты transaction CAS двух конкурентных выдач, лимит без обхода через API, отключение общего credential, индивидуальная подписка, отзыв/освобождение слота, disabled/expired parent, persistence/apply_failed/retry, отсутствие retired credentials в generated config, суммирование TCP/XHTTP/retired aliases без потери прежних байтов, encrypted v7, legacy/stale restore без отмены местного отзыва и компенсация неудачного удаления. Engine apply в новых lifecycle-тестах использует явно synthetic executable, это не реальный VPN.

После общего прогона изменились только HTTP middleware/test и frontend layout. Второй source tar SHA256: `58c795337dd9240992cd98036847a9af6ebd454a496a83668044f4fc22283dc5`. На нём повторены только затронутые HTTP проверки (exit 0):

```sh
go test ./internal/httpapi
go vet ./internal/httpapi
go test -race ./internal/httpapi -run TestDeviceAPIAuthorizationRetirementAndRevocation -count=1
```

В том числе anonymous/CSRF/CAS/legacy export denial, no-store и безопасный panic log: route pattern вместо secret-bearing URL. Первоначальный macOS запуск обнаружил отсутствующий Linux-only qrencode linker target и непригодный длинный macOS Unix-socket path; финальные проверки выполнены на Linux, без ослабления production checks.

## Frontend и браузер

`npm run build` (TypeScript/Vite, 140 modules) и `npm test` (11 tests) прошли под Node 22.16.0. После изменений layout повторена сборка. Chromium/Playwright работает с реальным disposable HTTP/auth/SQLite, без подмены ответов API. Stand — opt-in `ARCVPN_DEVICE_BROWSER=1 go test -v ./internal/httpapi -run '^TestDevicesBrowserFixture$' -count=1 -timeout=3m`, автоматически завершается через fixture-only endpoint/timeout; fixture имеет синтетические credentials, рабочий Xray отсутствует. Обычный `go test` пропускает stand, browser запуск выполняется отдельно.

`tests/devices_e2e.mjs` проверяет предзаполнение/предупреждение перехода, отключение общей подписки, лимит и прямую попытку обхода API, индивидуальную подписку, отзыв/404/повторную выдачу слота, CAS, правдивый pending apply, disabled common-key button, отсутствие JS errors. Проверены 320/390/768/1024/1440 в светлой и тёмной темах; поле подписки занимает доступную ширину. Первый визуальный просмотр выявил горизонтальное сжатие вложенного блока; исправлено существующими dialog-body/protocol-grid стилями, повтор browser PASS. Результаты скриншотов остаются в `/private/tmp/arcnet-usage-check/device-final/tests/screenshots/devices/`.

Финальный delivery tar SHA256: `6457b208f77737d120f849e773848f33617423c1df62bd6584ede39ea8145d67`. После него изменён только тест `internal/app/devices_test.go` (SHA256 `6564aa97638ee31130ab33a11dfe479f1d56feaebd8bcc00b5de43b46375ff0e`): CAS версии пакета читается после restore вместо ошибочного предположения о версии 0. По итоговому source + этому test overlay прошли `go test ./internal/store ./internal/app`, `go vet ./internal/store ./internal/app`, `go test -race ./internal/store ./internal/app` и `go build -o /output/devices-arcvpn ./cmd/arcvpn`, exit 0. Добавлена точная компенсация device state при неудачном импорте, с отдельным regression: новый token после ошибки не остаётся в DB. Прежние проверки остальных неизменённых Go модулей повторно не запускались.

Финальная frontend-сборка PASS. Последний browser regression **PASS** также проверяет сохранение введённого лимита после конкурентного CAS/refetch; прежний внешний layout пройден повторно. Финальные screenshots: `/private/tmp/arcnet-usage-check/device-delivery/tests/screenshots/devices/`. После этих проверок изменялась только документация.

## Не проверено этим блоком

Нет VPS/SSH/systemd/REALITY disconnect и overshoot-приёмки, физического HWID attestation, remote/UDP devices, нового per-device traffic/online источника, каскадов/runtime failover. Не устанавливались на production. После неудачного применения старый VPN может продолжаться; UI это явно показывает.
