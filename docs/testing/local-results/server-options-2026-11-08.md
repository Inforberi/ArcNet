# Серверные options — 8 ноября 2026

Текущее рабочее дерево. Linux amd64, existing arcnet-remote-tests:20261106, /src + existing Go cache. Frontend Node22.16.0. Без релиза/VPS.

- go test ./internal/store ./internal/protocols ./internal/xray ./internal/app ./internal/httpapi ./internal/backup -run 'ServerOptions|ClientOptions|ConnectionEdit|AuxiliaryEngineFailure|Fingerprint' -count=1 -timeout 90s: первые5 packages PASS. Backup FAIL: новая тестовая Hysteria-запись не имела обязательного credential. Fixture исправлена созданием credential, проверки не ослаблялись.
- go test ./internal/backup -run 'ServerOptions|ClientOptions|Fingerprint' -count=1 -timeout 30s: PASS.
- npm test:28/28 PASS. npm run build: TypeScript/Vite PASS. git diff --check PASS.

Покрыто: defaults/kind allowlist/timeout bounds, storage/read/list/restore, sniffing TCP/XHTTP JSON, Hysteria JSON и сохранение auth/TLS, API CSRF/CAS и server apply, equal nullable boolean, client-only update при запрещённом engine executable, combined rollback runtime/keys/server+client options, encrypted backup17/legacy16/rebind, UI diff.

NOT RUN: настоящий Xray/Hysteria config validation/handshake, VM/systemd/network rollback, VPS, browser/mobile/keyboard визуальная приёмка (Work settings). Mock preview18394 не доказывает VPN. Backup17 не читается прежними сборками.
