# ArcVPN stage4 0.6.1 — фактические локальные результаты (2026-11-04)

Основание: исходники после продолжения 0.6.0. Это наблюдения из рабочего окружения без реального VPS, без внешнего VPN-клиента и без npm-доступа.

| Проверка | Результат | Замечание |
|---|---|---|
| `go test -count=1 ./...` | PASS | все Go-пакеты, включая HTTP API/store/nodebootstrap |
| `go vet ./...` | PASS | завершился с кодом 0 |
| `go build -o /tmp/arcvpn-stage4-snapshot ./cmd/arcvpn` | PASS | `version` → `arcvpn 0.6.1-stage4`; dev бинарник без production React |
| `go test -race -count=1 ./internal/store ./internal/nodeagent ./internal/noderuntime ./internal/nodebootstrap ./internal/httpapi` | TIMEOUT (НЕ PASS) | store/agent/runtime/bootstrap PASS; полный HTTP API race не завершился |
| `go test -race -count=1 -run '^TestStage4' -timeout 50s ./internal/httpapi` | PASS | изолированные stage4 HTTP API тесты |
| `bash -n install.sh`, `scripts/*.sh`, `deploy/*.sh` | PASS | только статическая проверка синтаксиса, никаких root действий |
| TypeScript 5.8.3 `transpileModule` на 16 TS/TSX файлах | PASS | 0 синтаксических диагностик; типы не проверены |
| `npm install --package-lock-only ...` | BLOCKED | DNS `EAI_AGAIN` в исходном окружении; lockfile нет |
| `npm ci`, typecheck, build, E2E | NOT RUN | npm dependencies недоступны |
| VPS, SSH и Xray systemd/internet, VPN handshake | NOT RUN | нет подключённой тестовой инфраструктуры |

Тесты подписанного обновления проверяют только версионирование, структуру скрипта и rollback branches, а не фактическое поведение systemd. Тесты удаления не запускают опасные команды на рабочем хосте.

После завершения внешних испытаний результаты записываются **отдельно** в `docs/testing/external-results/` с конкретной командой, exit code, конфигурацией стенда и обезличенными логами.
