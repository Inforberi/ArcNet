# ArcVPN 0.6.2-stage4 — исправления, продолжение этапа 4

**Дата:** 2026-11-04. **Статус:** `STAGE4_SOURCE_IN_PROGRESS / GO_LOCAL_PASS / REACT_BUILD_BLOCKED / EXTERNAL_PENDING`. Это исходники для продолжения испытаний, **не production-релиз** и **не приёмка этапа 4**. База: проверенный SHA-256 архив `arcvpn-0.6.1-stage4-expanded-full-source.zip`.

## Реально изменённый код

| Область | Изменение | Граница доказательства |
|---|---|---|
| Безопасность установки | `internal/nodebootstrap/install.go`: после копирования из SSH-writable staging файлы в root-owned `/opt/arcvpn-node/bin` проверяются по SHA-256 **фактических бинарников подписанного пакета** до создания/старта служб. Предотвращена гонка между проверкой подписи и подменой файлов stage | Генерация скрипта, `sh -n` и unit-тесты; **реальный VPS не проверен** |
| Безопасность обновления | `internal/nodebootstrap/upgrade.go`: SHA-256 кандидатов проверяется после копирования в root-owned `.agent-next`/`.xray-next` **до** запуска тестовой версии Xray. Неудачная проверка кандидата не перезапускает действующий Xray; после начала переключения выполняется попытка отката обоих бинарников и служб | Shell/unit tests; systemd/kill/power-loss **NOT RUN** |
| Потеря SSH-ответа | `internal/nodebootstrap/install.go`, `upgrade.go`, `lifecycle.go`: после неоднозначного SSH отказа контроллер может read-only сверить install marker, версию, бинарные хеши, systemd; для detach/purge — конечные managed-ресурсы. Только полностью подтверждённое локальное состояние принимается как успех. Неполное/неизвестное — ошибка и проверка VPS | Генерация скриптов/локальные тесты; реальные сетевые сбои **NOT RUN** |
| Диагностика SSH | `internal/nodebootstrap/ssh.go`: коды ошибок SSH/timeout без раскрытия stdout/stderr. Выполнена проверка, что секрет из stderr не попадает в ответ | Go tests |
| Read-only preflight | `internal/nodebootstrap/{ssh,local}.go`, `internal/httpapi/node_bootstrap.go`: дополнительно выводит статус UFW, предупреждает про nftables/cloud firewall; проверяет с VPS HTTPS GET `/healthz` контроллера без `-k`, редиректов и токена. Недоступность HTTPS/неожиданный HTTP блокирует установку; отсутствие curl отображается как **непроверенный** фактор | Сценарии ответов curl имитированы локально; реальный VPS и сетевой firewall **NOT RUN** |
| Управление выдачей ссылок | SQLite `node_control.draining` с миграцией существующих 0.6.1 БД, `POST /api/nodes/{id}/drain`, идемпотентное повторное `resume` без непреднамеренного снятия `published`, React-кнопки, блокировка создания новых диагностических прямых ссылок при `draining=true` | Store/API/CSRF тесты; React TSX-синтаксис; browser **NOT RUN** |
| Browser E2E | `tests/react_e2e.py` обновлён под фактический UI этапа 4: выдача токена, регистрация агента, скрытие токена, drain + запрет новых ссылок | Python syntax PASS; Playwright на реальном React **NOT RUN** |
| Версия | `internal/nodeagent.Version`, дефолт `scripts/build-release.sh` и README — `0.6.2-stage4`; подписываемый пакет обязан иметь ту же версию, что и реальный бинарник | Go CLI сборка; подписанный production release **NOT BUILT** |

## Контракт drain/unpublish

`POST /api/nodes/{id}/drain` (админская сессия, CSRF) принимает **строго** JSON `{ "draining": true }` или `{ "draining": false }` и возвращает объект `NodeControl`. `draining=true` атомарно устанавливает `published=false`; новые запросы `GET /api/nodes/{id}/links` получают HTTP 409. Текущий Xray, действующие соединения, сохранённые на устройствах ссылки и heartbeat **не меняются**. Это **не** реальный drain сетевых сессий и **не** отзыв VLESS-учётных записей. Возврат к `draining=false` допускается только после подтверждения текущей ревизии; он **не** публикует endpoint, `vpn_probe` остаётся `unknown`. Автоматической публикации в подписках пока нет.

## Изменённая последовательность preflight/install/upgrade

1. Контроллер использует закреплённый Ed25519 SSH fingerprint, адрес публичного VPS, проверенный подписанный release. Private key находится только в ограниченном по времени временном файле, agent enrollment передаётся по SSH stdin.
2. До установки выполняются read-only OS/arch/CPU/RAM/disk/systemd/sudo/порт/accounts/paths, статус UFW, предупреждение по nft/cloud firewall и отдельная проверка исходящего HTTPS `/healthz` с VPS. `curl` отсутствует → `controller_https=unverified_no_curl`, **не PASS**.
3. Оператор подтверждает конкретный план изменений; скрипт устанавливает файлы в root-owned path и сверяет SHA-256 уже там. При ошибке выполняет ограниченную компенсацию только своих объектов, без автоматической модификации sshd/firewall.
4. При обновлении stage проверяется **до** запуска candidate Xray; неудачный кандидат не трогает действующий процесс. При ошибке после начала остановки служебных компонентов выполняется попытка отката. При неоднозначном SSH-ответе контроллер проверяет версии, хеши и systemd вместо оптимистического успеха.
5. После установки требуются реальный apply/ACK, проверка Xray и **независимый клиентский VPN handshake**. HTTPS `/healthz` подтверждает только доступность контроллера с VPS, не VPN-port ingress/egress.

## Открытые задачи, до `LOCAL_PASS` и `STAGE4_ACCEPTED`

- **P0 React:** настоящий npm `package-lock.json`, `npm ci`, `npm run typecheck`, `npm run build`, Playwright/browser E2E и проверка адаптивов. `npm install --package-lock-only` вновь завершился `EAI_AGAIN` npm registry в текущем окружении. **Не подделывать lockfile**.
- **P0 VPS:** чистая VM Debian/Ubuntu + отдельный клиент, signed install/upgrade/detach/purge, systemd service/permissions, UFW/cloud firewall, SSH nonstandard port, wrong signature/staged file substitution, abrupt SSH drop, power loss, rollback и реальный VLESS Reality handshake.
- **P1 source:** независимый VPN handshake-probe и политика публикации endpoint, UI confirmation всех lifecycle-сценариев, расширение negative/fault injection на настоящей VM, проверка frontend type errors. Существующий `drain` не блокирует уже выданные ссылки.
- **Исторический долг:** этапы 1–3 `EXTERNAL_PENDING`, SSH-hardening UI этапа 3 не реализован; это не подменяется SSH-wizard этапа 4.

**Источники истины:** [план](planning/stage4-checklist.md), [локальный тестовый протокол](testing/local-results/stage4-2026-11-04-0.6.2.md), [внешняя приёмка](testing/stage4-codex-handoff.md), [инструкция](ops/stage4-nodes.md). Не записывать `STAGE4_ACCEPTED` без реальных результатов в `docs/testing/external-results/`.
