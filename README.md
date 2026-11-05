**Решение от 2026-11-05 — текущих VPN-протоколов достаточно:** VLESS + REALITY TCP, VLESS + REALITY XHTTP, Hysteria2, WireGuard и AmneziaWG 3.1. **Этап 6 отложен**: AnyTLS, Trojan, TUIC, Shadowsocks 2022, экспертный режим и расширенные overrides сейчас не разрабатываются. Это не означает успешной VPS/E2E приёмки протоколов. [Запись решения](docs/planning/protocol-scope-freeze-2026-11-05.md).
# ArcVPN 0.7.0 — база этапа 5 (не production)

**Последнее локальное продолжение 2026-11-05:** добавлены [экономный дашборд серверов](docs/features/dashboard-and-telemetry.md) и [команды settings/password/session recovery](docs/ops/local-admin-recovery.md). Контрольный профиль пользователя — 512 МиБ/1 vCPU; [протокол локальных проверок](docs/testing/local-results/stage5-2026-11-05-dashboard-recovery.md) не заменяет VPS/VPN acceptance. После обновлённой установки: `sudo arcvpn settings`, `sudo arcvpn password reset`, для утраченной 2FA — явный `--reset-2fa`.

**Актуально 2026-11-05:** добавлены draft-only RoutePolicy, проверка графа и симуляция отказов, SQLite/API/React редактор маршрутов. Продолжение: интерактивные стрелки, форма/таблица-карточки на мобильных, защита зависимых нод и encrypted backup v4. Настоящие typecheck/Vite build и новый Chromium UI/API E2E проверены; полный внешний VPN gate остаётся открытым. **Каскады и failover на реальном Xray ещё не реализованы.** Этап 4 также не принят на реальных VPS.

Текущий блок: [результат UI/backup](docs/release-0.7.0-stage5-routing-ui.md), [фактические проверки](docs/testing/local-results/stage5-2026-11-05-routing-ui.md).

Начать с [результата stage5](docs/release-0.7.0-stage5-foundation.md), [чек-листа](docs/planning/stage5-checklist.md), [Codex handoff](docs/testing/stage5-codex-handoff.md) и [отчёта локальных тестов](docs/testing/local-results/stage5-2026-11-05-foundation.md). Для source-проверки: `./scripts/verify-stage5-source.sh`. `--full` выполняет `npm ci`, тесты модели, TypeScript и Vite build по добавленному `frontend/package-lock.json`. Go-скрипт предназначен для Linux с CGO/SQLite/QR libraries.

---

## Историческая точка: этап 4 (0.6.5)

# ArcVPN 0.6.5-stage4 — полный исходный проект

**Текущий статус (2026-11-04): `SOURCE_CHECKS_PASS / FRONTEND_BLOCKED / VPS_PENDING / STAGE4_NOT_ACCEPTED`.** Исходники и новые Go/security/fault-тесты пройдены, но это **не** принятый этап 4 и **не** production-релиз. Причина: без настоящего npm lockfile не выполнены React typecheck/build/E2E; необходимы реальные VPS и независимый VPN-клиент. Никаких заменений E2E на TS-синтаксис.

**Читать в первую очередь:** [результат 0.6.5](docs/release-0.6.5-stage4-source-audit.md), [проверки](docs/testing/local-results/stage4-2026-11-04-0.6.5.md), [матрица каждого S4](docs/planning/stage4-traceability-0.6.5.md), [внешняя приёмка](docs/testing/stage4-codex-handoff.md), [инструкция](docs/ops/stage4-nodes.md). Ниже оставлена история прежних поставок; её версии не являются текущей точкой разработки.

## Исторический контекст: архив 0.6.3 (не актуальный релиз)

**Статус на 2026-11-04:** этап 4 продолжен, но **не принят и не готов к production**. Go-тесты и статический анализ проходят; React typecheck/build и реальная установка на VPS с внешним VPN-handshake не подтверждены. Не устанавливать поверх действующей 3x-ui или production-системы.

**Текущие документы:** [изменения 0.6.3](docs/release-0.6.3-stage4-continuation.md), [статус проекта](docs/implementation-status.md), [текущая задача](docs/ai/current-task.md), [чеклист этапа 4](docs/planning/stage4-checklist.md), [инструкция Codex / VPS](docs/testing/stage4-codex-handoff.md), [инструкция по нодам](docs/ops/stage4-nodes.md).

Это **полные исходники**, включая Go backend, React/TypeScript frontend, агент, signed-release/SSH/CLI установку, удаление, rebind, upgrade, тесты и документацию. Это **не** полный бинарный релиз и **не** подтверждение клиентского VPN-подключения.

## Поддерживаемые сейчас контуры по исходникам

- Локальная панель: Go/SQLite, сессии, MFA, HTTPS, резервное копирование, протоколы и обновления из этапов 1–3; их внешний gate всё ещё открыт.
- Удалённые ноды: enrollment, персональный node secret, HTTPS heartbeat/poll/ACK, ревизии Xray, last-good и ограниченный root-helper. Только управляемый VLESS Reality TCP inbound; не каскады (этап 5).
- Установка: SSH wizard с pinning Ed25519 fingerprint, read-only preflight, подписанный ZIP агента/Xray, установка через stdin-токен. Альтернатива — локальный `arcvpn node-install` с заранее доверенным бинарником и отдельно подписанными файлами.
- Обслуживание: `node-remove --mode detach|purge`, `node-rebind`, SSH uninstall и подписанное повышение версии узла, операции и статусы в журнале. SSH/firewall чужих служб не изменяются автоматически.

## Локальный запуск разработки

```bash
bash scripts/dev-local.sh
```

Требуется окружение с Go, CGO/SQLite3, Node.js и npm dependencies. Скрипт запускает Go API на `127.0.0.1:8181` и Vite на `127.0.0.1:5173`; не устанавливает реальный VPN.

## Локальные Go-проверки

```bash
go test -count=1 ./...
go vet ./...
go build -o /tmp/arcvpn-dev ./cmd/arcvpn
bash -n install.sh scripts/*.sh deploy/*.sh
```

Фактические результаты: [протокол продолжения](docs/testing/local-results/stage4-2026-11-04-0.6.3.md). Проверка синтаксиса TSX не равна React typecheck.

## Production-сборка и внешняя приёмка

В исходном архиве **нет реального `frontend/package-lock.json`**: npm registry был недоступен (`EAI_AGAIN`). На сетевой доверенной Linux amd64 CI-машине:

```bash
cd frontend
npm install --package-lock-only --ignore-scripts --no-audit --no-fund
npm ci
npm run typecheck
npm run build
cd ..
ARCVPN_BUILD_VERSION=0.6.5-stage4 ./scripts/build-release.sh
./dist/arcvpn-linux-amd64 frontend-check
python3 tests/react_e2e.py
```

Сохранить lockfile в репозитории, закрепить verified Xray amd64, подписать agent+Xray на доверенной release-машине и только затем запускать проверки из `docs/testing/stage4-codex-handoff.md` на чистом VPS. После выполнения испытаний зафиксировать действительные результаты в `docs/testing/external-results/`.

## Ключевые ограничения

`agent online` и `Xray active` не доказывают доступность VPN, `vpn_probe=unknown` и `published=false` до независимого тестирования. Подписанный ZIP удостоверяет происхождение файлов относительно trust anchor, **но не совместимость бинарников**. Нет mTLS, проверенного на реальном Xray внешнего VPN probe, готового UI E2E, безопасного автоматического SSH/firewall hardening и подтверждённого восстановления после реального обрыва SSH во время обновления.

Исторические handoff документы: [этап 3 (0.5.1)](docs/release-0.5.1-stage3-code-handoff.md), [этап 4 (0.6.0)](docs/release-0.6.0-stage4-source.md). Актуальное состояние — **0.6.5-stage4**, не исторические версии.


## Изменения 0.6.3: тест с независимого клиента

Добавлена **возможность** внешнего теста VLESS/REALITY. На доверенной независимой Linux-машине необходимо установить проверенный Xray и Go CLI ArcVPN этой версии, подготовить приватный файл 0600 с тестовой ссылкой. В админке «Серверы» → «Тестовые ссылки» → «Создать одноразовый допуск», затем выполнить CLI `arcvpn node-probe --link-file ... --xray ... --controller ... --node-id ... --revision ... --digest ... --report-token-stdin` с **реальными значениями из панели**, не передавая токен или VLESS-ссылку в аргументах. Оператор должен самостоятельно контролировать место выполнения. Проба проверяет единственный HTTPS endpoint через VPN, не является криптографическим подтверждением места запуска, не публикует ноду автоматически. Детальная инструкция: [stage4-nodes.md](docs/ops/stage4-nodes.md).
