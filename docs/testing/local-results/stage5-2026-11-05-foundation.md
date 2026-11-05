# Локальные проверки базы этапа 5 — 2026-11-05

**Тестируемая конфигурация:** код из архива исходников `0.6.5` + новые изменения `0.7.0-stage5-foundation`.

| Проверка | Результат | Ограничение |
|---|---|---|
| `go test -count=1 ./...` | PASS | Включает новые domain/store/API тесты и существующую регрессию, без реальных VPS |
| `go vet ./...` | PASS | Статический анализ Go |
| `go test -count=1 ./internal/routes ./internal/store ./internal/httpapi` | PASS | Domain, миграции, HTTP auth + CSRF |
| TypeScript transpile/syntax (16 файлов) | PASS | Не `tsc --noEmit`/production build |
| `go test -count=1 -race ./internal/routes ./internal/store` | PASS | Выполнено скриптом `scripts/verify-stage5-source.sh`, не race всего HTTP API |
| `go build ./cmd/arcvpn` (вывод во временный каталог) | PASS | Обычный CLI, не production bundle с React |
| React `npm ci`/typecheck/Vite | NOT RUN | `frontend/package-lock.json` отсутствует; не выдавать за PASS |
| Реальный Xray data-plane cascade | NOT IMPLEMENTED | В этом срезе только безопасный draft и симуляция |
| Реальный VPN-handshake и egress IP | NOT RUN | Требуются VPS и независимый клиент |

Итог: `SOURCE_TESTS_PASS`, а не `LOCAL_PASS` или `STAGE5_ACCEPTED`.
