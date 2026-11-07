# Первый вход и подключение — 2026-11-07

## Подтверждено локально

- Node 22.16.0: `npm --prefix frontend run build` — TypeScript/Vite PASS после финальных UI/SNI изменений.
- Linux fixture image `arcnet-remote-tests:20261106`: `go test ./internal/store ./internal/httpapi ./internal/xray ./internal/app ./cmd/arcvpn` — PASS.
- После SNI изменений: `go test ./internal/targetcheck ./internal/publicnet ./internal/store ./internal/httpapi ./cmd/arcvpn` — PASS.
- `go test -race ./internal/store ./internal/httpapi -run 'TestAdmin|TestChangeAdmin|TestHTTPMFA|TestTarget|TestDeviceDirect' -count=1` — PASS. Покрыты одноразовый claim/повтор/expiry, origin/CSRF, отдельный MFA-шаг без сессии, смена credentials с отзывом сессий и сохранением MFA, job reuse, device QR/revoke.
- Настоящий Linux arm64 Xray 26.6.1, сеть контейнера отключена: `ARCVPN_TEST_XRAY=/fixtures/xray go test -v ./internal/xray -run 'TestRealRealityConfiguration|TestGenerateKeysAccepts' -count=1` — PASS после исправления `Password (PublicKey)`. Реальные keygen/config-test, без VPN handshake.
- `bash -n install.sh deploy/arcvpn-menu.sh` — PASS; CLI submenu/setup fixture входит в cmd tests.
- В браузере на настоящем React/API с временной SQLite наблюдались: регистрация со своим паролем → вход без лишнего TOTP → пустая таблица → диалог Hysteria2 → созданная строка → пользователь → диалог подписки/QR/Happ/Streisand. Движки simulated. Снимок `/private/tmp/arcnet-connections-preview.png`, только синтетические данные. Процесс opt-in browser fixture позже завершился своим 180-секундным timeout; его завершение НЕ PASS и не серверная приёмка.

## VPS: только показания оператора

Старая A4: панель active после исправления прав, но `arcvpn doctor` сообщает `VPN: not configured`; Xray unit inactive. Установленный Xray 26.10.31 выводит `PrivateKey`, `Password (PublicKey)`, `Hash32`; значения скрыты на VPS. Это подтверждает несовместимость старого парсера, но не применение исправления на сервере.

Новая регистрация/UI/core на VPS ещё не установлены. Реальный клиентский VPN-трафик, мобильный финальный UI, полный upgrade/rollback, reboot, renewal и тесты Happ/Streisand NOT RUN. Существующие 3x-ui, данные администратора и сетевые настройки не менялись этим исходным блоком. Повтор install поверх A4 запрещён.
