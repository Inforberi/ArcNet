# ArcVPN 0.4.0 stage2 source candidate — 2026-11-04

**Не production. Не заменять действующую 3x-ui.** Исходники и автоматические проверки; нет проверенной сборки React, реального VPN E2E и тестового VPS.

## Реализованная кодовая часть

- SQLite схемы `protocols` + `protocol_credentials`; per-user UUID/пароли/ключи, отдельные подсети WG/AWG.
- VLESS Reality TCP + XHTTP в Xray конфигурации, Hysteria2 YAML userpass/TLS; WireGuard и AWG 3.1 конфиги сервера и пользователей.
- REST создание, листинг, включение/отключение и удаление профилей. Проверка портов, запрет конфликтов TCP с UI/ACME, запрет некорректного Hysteria SNI, безопасные ключи.
- Аутентифицированный экспорт URI, `.conf` и QR; базовая подписка VLESS/XHTTP/Hysteria2; отдельный `.conf` для WG/AWG.
- React UI использует фактические API, а не демонстрационные переключатели. Local systemd-status endpoint не притворяется клиентской сетевой проверкой.
- Вспомогательные движки: генерация и применение конфигов, rollback, whitelisted Unix socket systemd helper. Root-owned промежуточный файл WG/AWG с проверкой директив; запрет shell-вставок.
- NAT/IPv4 forwarding, backup v2 с приватными ключами и восстановлением, совместимость с backup v1, проверки ссылок и прав админа.

## Локально действительно выполнено

- `go test ./...` — PASS.
- `go vet ./...` — PASS.
- `go test -race -count=1 ./internal/app ./internal/httpapi ./internal/backup ./internal/protocols ./internal/engines ./internal/store` — PASS.
- `bash -n install.sh deploy/arcvpn-tunnel-net.sh scripts/build-release.sh` — PASS.
- TypeScript/TSX AST синтаксис 16 файлов — PASS; **не typecheck**.
- HTTP интеграционный тест реальных Go API с имитацией Xray CLI и локального engine manager; тесты не устанавливают настоящие Xray/Hysteria/WG/AWG.

## Обязательные блокеры и ограничения

- React `npm ci`, TS typecheck, Vite production build и браузерный E2E здесь не выполнены: registry и браузерная навигация ограничены. Отсутствует `frontend/package-lock.json`.
- Runtime AWG 3.1 **не установлен установщиком**; понадобится подтверждение совместимого ядра, инструментов и подписанного способа поставки.
- Не проверены реальный VPS, Xray/Hysteria2/WG/AWG бинарники, поддержка XHTTP версиями клиентов, TLS, QUIC/UDP, NAT/UFW, SSH, реальные подписки.
- Автоматическое изменение облачного firewall/UFW через UI, управление произвольными inbound overrides и каскады относятся к следующим этапам.
- Нет подтверждения целевых 1 vCPU/1 ГБ RAM, отсутствия DPI-обнаружения или бесшовного failover.

**Моя часть этапа 2: НЕ ВЫПОЛНЕНА полностью**, потому что автоматическая поставка AWG и проверенная React production-сборка не закрыты. Бóльшая часть серверной логики реализована и локально проверена. **Проверка Codex: ОЖИДАЕТСЯ.**
