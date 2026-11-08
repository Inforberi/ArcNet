# Клиентские параметры — 8 ноября 2026

Текущее рабочее дерево, локально, без релиза/VPS. Linux amd64: существующий образ arcnet-remote-tests:20261106; команды выполнялись через docker run --rm, project /src, existing Go cache. Frontend Node22.16.0, зависимости установлены ранее.

Выполнено:

- go test ./internal/store ./internal/protocols ./internal/app ./internal/httpapi ./internal/backup -run 'ClientOptions|Fingerprint|TestEditConnection' -count=1 -timeout 90s: store/protocols/httpapi/backup PASS; app в этом вызове только compile, no tests to run.
- go test ./internal/app -run 'ConnectionEdit|AuxiliaryEngineFailure' -count=1 -timeout 60s: PASS, существующие edit/identity/paused/rollback.
- go test ./internal/store ./internal/app ./internal/backup -run ClientOptions -count=1 -timeout 60s: PASS, включая combined server edit failure и backup WG keepalive0.
- go test ./internal/store -run ClientOptions -count=1 -timeout 30s: PASS после ограничения DNS IPv4.
- go test ./internal/httpapi -run '^TestClientOptionsAPIAndSubscription$' -count=1 -timeout 30s: PASS, CSRF403, validation422, stale409, mode в подписке, engine call запрещён fake executable.
- npm run build: TypeScript/Vite PASS; npm test:27/27 PASS; git diff --check PASS.

Проверены defaults, mode allowlist, protocol ownership, integer bounds, config newline injection, read/list/write/delete/restore/transaction rollback, encrypted backup16 + read15, сохранение keys и runtime, клиентский diff с явным0. Export-only не запускает движок и не читает пользователей.

NOT RUN: реальные WG/Xray/Happ/Streisand handshake, browser/mobile/keyboard visual acceptance (Work settings), VM/systemd/IPv6/firewall и release. Последние Go проверки — fixtures, не сетевой PASS. UI mock18394 перезапущен на новом backend, не является реальным VPN. Backup16 не читается прежними сборками.
