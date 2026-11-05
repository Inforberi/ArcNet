# ArcVPN 0.6.4-stage4 — единая сдача исходников, аудит безопасности

**Дата:** 2026-11-04. **Состояние:** `GO_PASS / CRITICAL_RACE_PASS / FRONTEND_BLOCKED / EXTERNAL_PENDING`. Этап 4 **не принят целиком**. Наличие API, backend и тестовых фикстур не заменяет настоящий React build, systemd, реальный Xray и VPN-handshake. Поддерживаемая форма поставки — полный source ZIP, а не production-бинарник.

## Исправлено в исходниках

1. `internal/nodeagent/agent.go`: чтение приватного `node.json` выполняется через один файловый дескриптор с `O_NOFOLLOW|O_NONBLOCK`, проверкой regular file и режима, лимитом 16 KiB; устраняется symlink-swap и чтение без ограничений. Тесты: `credentials_security_test.go` (symlink, permissions, oversized input, FIFO).
2. `internal/noderuntime/runtime.go`: повтор того же `operation_id/revision/sha256` теперь возвращает `applied` только при совпадении действующего файла Xray с отправленной конфигурацией **и** после успешного `systemctl is-active`. Повреждённый receipt блокирует новые операции, вместо сброса монотонности ревизий. Config и receipt читаются с `O_NOFOLLOW` и ограничением размера; тесты на повреждённый конфиг, выключенный service, symlink, malformed receipt.
3. `internal/nodebootstrap/install.go`: подписанный ZIP читается через один no-follow файловый дескриптор и `MaxArchive` limit; rollback удаляет системных пользователей **только если текущая установка успела создать их**. Не отменяет изменения `apt` и не означает успешный systemd rollout. Тесты: `archive_security_test.go`.
4. `internal/updater/updater.go`: публичный и приватный signing keys читаются с no-follow и size limit; для приватного ключа запрещены права доступа группы и остальных пользователей. Тесты проверяют symlink, exposed key и oversized file.
5. `internal/nodeprotocol/origin.go`: общее строгое правило для HTTPS-origin контроллера (`nodeagent`, local/SSH bootstrap): корректный DNS/IP, порт 1–65535, отсутствие пути/query/userinfo и shell-опасных символов. `frontend/src/pages/Nodes.tsx`: однотипное shell-quoting динамических строк в отображаемых CLI-инструкциях. Нет отправки секретов через аргументы процесса.
6. Полная интеграция сохраняет исходный интерфейс HTTP API/SQLite и текущие ограничения `published=false`. Прямые VPN-ссылки, `drain` и выданные grant не становятся автоматически подтверждённым клиентским соединением.

## Локальная проверка

Фактические команды и вывод: `docs/testing/local-results/stage4-2026-11-04-0.6.4.md` и `.log` рядом.

- `go test -count=1 ./...` — PASS.
- `go vet ./...` — PASS.
- `go build -o /mnt/data/arcvpn-stage4-final-cli ./cmd/arcvpn` — PASS; **CLI без встроенной production React**.
- `go test -race -count=1` по `nodeagent`, `nodebootstrap`, `nodeprotocol`, `noderuntime`, `updater`, `store`, `nodeprobe` — PASS.
- `go test -race -count=1` по выбранным критическим `httpapi` сценариям — PASS; **полный** race прогон всего HTTP API не подтверждён.
- `bash -n install.sh scripts/*.sh deploy/*.sh`, `python3 -m py_compile tests/react_e2e.py` — PASS.
- Синтаксический TypeScript parser (17 файлов, TS 5.8.3) — PASS, но это **не typecheck**.
- npm registry не разрешается из окружения, `frontend/package-lock.json` и `node_modules` отсутствуют. `npm ci`, `npm run typecheck`, production React build и Playwright E2E — **BLOCKED / NOT RUN**.
- Реальный Xray `run -test`, SSH clean install, rollback в systemd, публичный TCP 443 и VLESS/REALITY handshake с независимой машины — **NOT RUN**.

## Незавершённая повторная проверка и требования исходного scope

`bash scripts/verify-stage4-local.sh --source` в комбинированном прогоне не завершился на выбранном race-пакете HTTP API до лимита сессии. Отдельный запуск этих тестов прошёл ранее. В скрипт внесены явные тайм-ауты; полное повторное прохождение после изменения тайм-аутов не подтверждено.

Ряд пунктов исходного `docs/planning/stage4-checklist.md` всё ещё не имеет полного документированного доказательства (в частности `S4-02`, `S4-10–13`, `S4-16`, `S4-20`, `S4-25–26`, `S4-30–36`, `S4-40–44`, `S4-50–51`), даже если часть функциональности присутствует в исходниках. Требуется формальная сверка реализаций и тестов с критериями.

## Ограничения, требующие внешнего стенда

- **P0:** получить корректный npm-generated lockfile на доверенной сетевой build-машине; `npm ci`, typecheck, Vite build, embedded React и Playwright на 320/390/768/1024/1440. Не писать lockfile вручную.
- **P0:** clean Debian/Ubuntu VM, подписанная установка/upgrade/detach/purge/rebind, UFW, custom SSH port, реальный запуск agent/helper/Xray, отказ питания, обрыв SSH и откат по проверяемым квитанциям.
- **P0:** независимый клиент с настоящим Xray и действующим VLESS/REALITY: подтверждённый handshake, внешний HTTPS egress, негативные сценарии, проверка недопустимости auto-publication. `pass` от bearer grant не является криптографическим доказательством запуска теста.
- **P1:** дополнительно проверить SSH-hardening как открытый долг этапа 3 (не смешивать с установщиком нод этапа 4), политику rebind/backup и стендовое восстановление.

**Gate:** `LOCAL_PASS` не выдавать без React build/typecheck/E2E; `STAGE4_ACCEPTED` не выдавать без результатов на реальном VPS и независимом клиенте. Результаты должны находиться в `docs/testing/external-results/` после реального выполнения. Исходники 0.6.4 не предназначены для установки на действующую production VPS.
