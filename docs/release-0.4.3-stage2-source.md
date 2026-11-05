# ArcVPN 0.4.3 — исправления надёжности этапа 2

Дата: 2026-11-04. **Исходники, не production; этап 2 остаётся открытым.**

## Что изменено

1. Генераторы клиента и сервера AmneziaWG 3.1 теперь используют `RandomTrailers = off`, оставляя `ContentPaddingAddition = 32-128` и `DisableCookies = off`. Root-валидатор требует такое же значение и отвергает подмену.
2. Причина: открытые upstream issues [#185](https://github.com/amnezia-vpn/amneziawg-go/issues/185), [#186](https://github.com/amnezia-vpn/amneziawg-go/issues/186), описывающие риск нестабильного padding/обработки пакетов с RandomTrailers. Это консервативная настройка, а не подтверждённое исправление всех дефектов upstream.
3. GitHub Actions больше не ломается до сборки из-за `setup-node cache` без lockfile. Временный bootstrap `npm install --package-lock-only --ignore-scripts` формирует lockfile и сохраняет его отдельным артефактом. Для воспроизводимого релиза lockfile должен быть добавлен в исходники и проверен.
4. Уточнены ограничения в текущей документации и передаваемые Codex проверки.

## Локальные проверки

- `go test ./...` — PASS.
- `go vet ./...` — PASS.
- `go test -race ./internal/engines ./internal/protocols ./internal/app` — PASS.
- `go test -race ./internal/httpapi` — PASS.
- `go test -race ./internal/backup` — PASS.
- `bash -n` установщика и release-скриптов — PASS.
- npm/Vite build, AWG upstream build, браузерный E2E на React и реальные VPN-соединения — **не выполнялись**.

## Осталось до завершения этапа 2

- Закрепить `frontend/package-lock.json`, выполнить `npm ci`, `npm run build`, React browser E2E и собрать Go с настоящими статическими файлами React.
- Собрать и проверить upstream AWG3.1 binaries на сетевом Linux runner с Go 1.25+.
- Проверить реальные соединения всех заявленных профилей, full-tunnel/NAT/DNS, отзыв клиентов, восстановления, systemd, откаты и установщик на отдельном VPS.
- Устранить обнаруженные ошибки и повторить испытания.

**Моя часть этапа 2: НЕ ВЫПОЛНЕНА ПОЛНОСТЬЮ. Проверка Codex: ОЖИДАЕТСЯ.**
