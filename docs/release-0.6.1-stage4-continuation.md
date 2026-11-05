# ArcVPN 0.6.1-stage4 — продолжение этапа 4 / полный исходный код

**Дата:** 2026-11-04. **Статус:** `STAGE4_SOURCE_IN_PROGRESS / GO_TEST_PASS / REACT_BUILD_BLOCKED / VPS_NOT_RUN`.
Это **кодовый снимок для проверки**, не production-релиз и не `STAGE4_ACCEPTED`.

## Исходная точка и границы

Основание: архив `arcvpn-0.6.0-stage4-source-preview-full.zip`. Файлы кода этапов 1–3 сохранены. В этом снимке нет собранного React, готового Xray для VPS, реальной SSH-сессии с тестовой VM, подтверждения VPN-handshake и внешней приёмки. Все эти пункты **NOT RUN**, а не `PASS`.

## Реализовано после 0.6.0

| Участок | Исходники и результат | Контроль |
|---|---|---|
| Совместимость управления | `internal/nodeprotocol/protocol.go`, миграция `managed_nodes.agent_protocol`, heartbeat v1, блокировка выдачи `apply_xray_v1` устаревшим/неидентифицированным агентам | Go unit+HTTP PASS |
| SSH preflight | Исправлена незакрытая кавычка исходного POSIX-скрипта. Read-only проверяет архитектуру, ОС, sudo, systemd, apt, RAM/disk, конфликт порта, аккаунты, каталоги и systemd units ArcVPN; чужой firewall только отображается | `sh -n` и тесты PASS; проверка реальной VM NOT RUN |
| Установка локальной командой | `cmd/arcvpn/main.go: node-install`, `internal/nodebootstrap/local.go`: отдельно проверенный Ed25519 ZIP+публичный ключ, preflight, stdin для одноразового токена, защищённый stage, ограниченный rollback при неуспешной регистрации | Go tests PASS, VM NOT RUN |
| Безопасное удаление | `internal/nodebootstrap/lifecycle.go`: `detach` оставляет Xray активным и удаляет управление; `purge` удаляет только известные ArcVPN-файлы, users и units. Корневой install marker должен совпасть с исходным ID | shell syntax и unit PASS; VM NOT RUN |
| SSH uninstall | `internal/httpapi/node_removal.go`, `internal/store/node_bootstrap.go`: отдельный plan+explicit approve, fingerprint/IP/port/marker pinning, persistent journal, отложенный revoke после удалённого действия | Go store + auth tests PASS; реальный SSH NOT RUN |
| Rebind | `internal/nodeagent/agent.go: Rebind`, CLI `node-rebind`: смена контроллера после однократной новой регистрации, сохранение old identity при сетевой ошибке, остановка/перезапуск agent без удаления Xray | TLS fixture+atomic file tests PASS; live rebind NOT RUN |
| Подписанное обновление | `internal/nodebootstrap/{upgrade,version}.go`, `internal/httpapi/node_upgrade.go`: совместимость 0.6.x, только увеличение patch, независимая подпись пакета, проверка оригинального SSH install marker и текущей версии, Xray `run -test` до замены, резерв двух бинарников и rollback при ошибке systemd | Go/unit/schema/`sh -n` PASS; реальный service rollback NOT RUN |
| Журнал и миграция | `node_bootstrap_operations`: `operation_type`, `removal_mode`, `target_version`; конкурирующие upgrade/remove блокируются в SQLite; перезапуск контроллера помечает running как failed | миграция старого журнала и конкурентные unit PASS |
| Обработка незавершённой установки | После сбоя enrollment SSH worker проверяет `node.json`: если identity заведомо отсутствует, пытается удалить только свою установку; при неопределённом результате сохраняет VPS для ручного разбора | скомпилировано и обычные Go PASS; VM fault injection NOT RUN |
| React-серверы | `frontend/src/pages/Nodes.tsx`: фильтры, управляемое удаление, SSH installer, upgrade wizard, журнал типов операций, раздельные состояния agent/Xray/VPN readiness | TSX parse PASS; TypeScript/Vite/React E2E NOT RUN |
| Подпись версии | `scripts/build-release.sh` stamps `nodeagent.Version` с версией 0.6.1; `scripts/build-stage4-node-bundle.sh` сверяет версию бинарника с подписываемым manifest | `bash -n`, Go build PASS; production bundle NOT BUILT |

## API контракты дополнения

Все административные вызовы требуют действующей сессии и CSRF. **Нет API для произвольного shell на агенте.**

- `POST /api/nodes/ssh/inspect` — получить SSH Ed25519 fingerprint; пользователь сверяет ключ через независимую консоль провайдера.
- `POST /api/nodes/ssh/preflight` — read-only план. `POST /api/nodes/ssh/install` — отдельное `CONFIRM_INSTALL`.
- `POST /api/nodes/{id}/ssh/remove-plan` с `credentials`, `mode` (`detach` или `purge`) → `installation_id`, `changes`.
- `POST /api/nodes/{id}/ssh/remove` дополнительно принимает `installation_id` и `approve` (`CONFIRM_DETACH` или `CONFIRM_PURGE`).
- `POST /api/nodes/{id}/ssh/upgrade-plan` с `credentials` → `installation_id`, `current_version`, `target_version`, `changes`.
- `POST /api/nodes/{id}/ssh/upgrade` дополнительно принимает `installation_id`, `expected_current`, `approve: CONFIRM_UPGRADE`. Контроллер повторно проверяет IP, SSH key, managed marker и установленную версию перед действием.
- `GET /api/nodes/ssh/operations` — единый persistent журнал `install`, `remove`, `upgrade`; новая версия `target_version`, а `status=upgraded` означает только проверенный локальный systemd результат, **не** внешний VPN-probe.

## Релизный процесс

1. На сетевой Linux amd64 CI-машине создать **настоящий** `frontend/package-lock.json` через `npm install --package-lock-only --ignore-scripts --no-audit --no-fund`. Добавить его в Git. Повторить `npm ci`, `npm run typecheck`, `npm run build`, `python3 tests/react_e2e.py` в браузерном окружении.
2. После этого выполнить `ARCVPN_BUILD_VERSION=0.6.1-stage4 scripts/build-release.sh`. Бинарник обязан отвечать на `arcvpn version` ровно `arcvpn 0.6.1-stage4`.
3. Получить и проверить подходящий Xray linux/amd64 из независимо подтверждённого источника; его точную версию и SHA-256 зафиксировать в release report. Подписать оба бинарника **офлайн** через `scripts/build-stage4-node-bundle.sh`, передав `0.6.1-stage4` как manifest-версию.
4. На контроллер перенести лишь подписанный ZIP и соответствующий pinned Ed25519 public key. Приватный ключ подписания никогда не копировать на VPS. Настроить `ARCVPN_NODE_RELEASE_BUNDLE` и `ARCVPN_NODE_RELEASE_KEY` только как пути к root-owned файлам.
5. Испытать `node-install`, UI SSH install, реальную конфигурацию и VPN-handshake с отдельного клиента; затем `upgrade` с 0.6.0→0.6.1, его отказ/rollback, `detach`, `purge`, `node-rebind` и recovery. Не тестировать на production.

## Реально выполненные локальные команды

- `go test -count=1 ./...` — **PASS**.
- `go vet ./...` — **PASS**.
- `go build -o /tmp/arcvpn-stage4-snapshot ./cmd/arcvpn` — **PASS**; `version` → `arcvpn 0.6.1-stage4`.
- `go test -race -count=1 ./internal/store ./internal/nodeagent ./internal/noderuntime ./internal/nodebootstrap` — **PASS**.
- `go test -race -count=1 -run '^TestStage4' -timeout 50s ./internal/httpapi` — **PASS**.
- Общий race-прогон **включая весь HTTP API** — **TIMEOUT / не удалось подтвердить PASS**; остальные пакеты успели завершиться успешно.
- `bash -n` для `install.sh`, `scripts/*.sh`, `deploy/*.sh` — **PASS**.
- TypeScript transpileModule parse всех 16 TS/TSX файлов — **PASS, 0 syntax diagnostics**. Это **не** TypeScript typecheck.
- `npm install --package-lock-only` из первоначального source 0.6.0 — **BLOCKED `EAI_AGAIN` npm registry**. `npm ci`, `npm run typecheck`, `npm run build`, React E2E — **NOT RUN**.
- Xray, реальные VPS/SSH/UFW/systemd, VPN handshake, crash/VM rollback — **NOT RUN**.

Детальный repeatable статус: `docs/testing/local-results/stage4-2026-11-04-continuation.md`.

## Блокеры до `STAGE4_ACCEPTED`

1. **React и сборочный gate:** настоящий lockfile, `npm ci`, production typecheck/build, визуальные/keyboard E2E на 320/390/768/1024/1440 и исправление обнаруженных ошибок.
2. **VPS gate:** чистая VM + второй VPS + независимый клиент; Debian/Ubuntu, SSH pinning, sudo, Xray `run -test`, порт 443, firewall/SSH сохраняются; проверить реальный VPN-handshake/egress, разрыв связи контроллера, reboot, upgrade rollback и безопасный uninstall.
3. **Fault-injection:** SSH обрыв в `stage`/`enroll`/`upgrade`, disk full, systemd fail, lost ACK, controller restart; подтвердить состояние файлов и journal на VM. Статус `failed` после controller restart означает **нужно проверить фактический VPS**, а не автоматический rollback.
4. **Source gaps:** расширенный diff preflight сети/firewall, полноценный E2E/доступность UI, drain/unpublish и независимый VPN-probe (без этого нет автоматической публикации), дополнительные нагрузочные/сбойные интеграционные тесты. Нельзя объявить интерфейс или этап завершённым по TSX syntax.
5. **Исходный долг:** этапы 1–3 `EXTERNAL_PENDING`, а SSH hardening из первоначального scope этапа 3 не реализован. Это отдельная задолженность, не скрытая stage4.

**Итог:** текущий блок продолжения кодовой разработки выполнен и упакован целиком. Сам этап 4 ещё требует программных и внешних проверок. Точная передача: `docs/testing/stage4-codex-handoff.md` и `docs/planning/stage4-checklist.md`.
