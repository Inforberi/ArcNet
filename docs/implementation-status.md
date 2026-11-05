> **Локальные пакеты Xray — 2026-11-05:** реализованы подтверждённые counters/checkpoints/debits, пакет/срок/новый период, UI/API и backup v6. [ADR-008](adr/008-local-xray-traffic-packages.md), [проверки](testing/local-results/stage5-2026-11-05-user-packages.md). Новая реализация не распространяет учёт на Hysteria2/WG/AWG/ноды и не завершает устройства или stage5 runtime. Systemd/VPS disconnect и byte overshoot не приняты.

> **Продолжение live monitoring:** default5, сохраняемый interval UI, SSE и формы с defaults локально проверены. [Результаты](testing/local-results/stage5-2026-11-05-live-monitoring.md). Учёт гигабайтов пользователя, device enforcement, первый любой протокол и каскадный runtime **не реализованы**; [план](planning/simple-operation-and-user-packages-2026-11-05.md).

> **Scope freeze 2026-11-05:** текущих пяти вариантов VPN-подключения достаточно; этап 6 отложен без разработки. Код поддерживаемых профилей присутствует, но реальный E2E статус не подтверждён. [Зафиксированное решение](planning/protocol-scope-freeze-2026-11-05.md).
# Текущий статус — 2026-11-05

- **Этап 4:** текущий React production build проверен и новый UI/API E2E пройден; исходный `tests/react_e2e.py`, VPS/systemd/SSH и независимый egress остаются отдельными gate. НЕ принят.
- **Этап 5:** 0.7.0 draft RoutePolicy + graph validator + SQLite/HTTP + simulation, интерактивные стрелки/форма/таблица, защита удаления зависимых нод и backup v4. Go tests/vet/race и frontend typecheck/build/routing E2E PASS. Xray runtime, публикация подписок и real failover **не реализованы**. [Объём](release-0.7.0-stage5-routing-ui.md), [доказательства](testing/local-results/stage5-2026-11-05-routing-ui.md).

[Развёрнутый status](release-0.7.0-stage5-foundation.md) · [План этапа 5](planning/stage5-checklist.md) · [Codex handoff](testing/stage5-codex-handoff.md).

---

## Историческая информация ниже

# Актуальный статус 0.6.5-stage4 (2026-11-04)

**Только исходники:** SOURCE_CHECKS_PASS. **Не выполнены** frontend npm/typecheck/build/E2E, VPS/systemd/Xray/VLESS-handshake. Не LOCAL_PASS и не ACCEPTED. Новое: ключ шифрования no-follow, bounded journals, IPv6 SCP, агент backoff, строгий CI lockfile. [Подтверждённые проверки](testing/local-results/stage4-2026-11-04-0.6.5.md); [матрица scope](planning/stage4-traceability-0.6.5.md). **Все статусы ниже исторические.**

---

# Текущий статус — 0.6.4-stage4 (2026-11-04)

**Проверенный факт:** Go tests/vet/build, selected race tests, shell/Python/TS syntax PASS; security code fixes and regression tests в 0.6.4. **Открыто:** npm/React production/typecheck/E2E (DNS npm), реальные SSH/VPS/systemd/Xray/VLESS проверки. Статус `SOURCE_HARDENED / FRONTEND_BLOCKED / EXTERNAL_PENDING`, не `LOCAL_PASS`, не `STAGE4_ACCEPTED`, не production. Подробности: [релиз 0.6.4](release-0.6.4-stage4-security-and-acceptance.md) и [фактические проверки](testing/local-results/stage4-2026-11-04-0.6.4.md).

## Историческое состояние 0.6.3-stage4 (2026-11-04)

**Факт:** независимый VLESS/REALITY probe **реализован в исходниках** как отдельный Xray+SOCKS5+HTTPS 204 CLI; добавлен одноразовый grant, TTL, hash, ревизия/digest, backend API, UI, миграции, тесты и сброс состояния на изменение ноды. `go test`, `go vet`, Go build, race-критических пакетов, Bash/TypeScript syntax — PASS; полный HTTP API race — TIMEOUT; `npm` — `EAI_AGAIN`, production React / Playwright / VPS / Xray real handshake **NOT RUN**. **Не LOCAL_PASS, не ACCEPTED, не production**. [0.6.3 handoff](release-0.6.3-stage4-continuation.md), [тесты](testing/local-results/stage4-2026-11-04-0.6.3.md), [остаток](planning/stage4-checklist.md).

# Текущий статус ArcVPN — 0.6.2-stage4 (2026-11-04)

**Факт:** выполнены доработки SHA-256 на root-owned бинарниках, SSH read-only recovery, egress HTTPS preflight, conservative UI/API drain и регрессионные Go тесты. `go test` / `vet` / shell syntax / TypeScript *syntax* PASS; выбранный `go test -race` PASS. Реальный `npm ci/typecheck/build` **BLOCKED `EAI_AGAIN`**, browser E2E и VPS/systemd/Xray/VPN **NOT RUN**. **Этап 4 всё ещё не завершён.** [Handoff 0.6.2](release-0.6.2-stage4-continuation.md), [локальный протокол](testing/local-results/stage4-2026-11-04-0.6.2.md).

---

# Текущий статус ArcVPN — 0.6.1-stage4 (2026-11-04)

**Факт:** кодовый снимок дополнен CLI node-install/remove/rebind, подписанным SSH upgrade, uninstall, протокольным version gate, SQLite migration/journal и React wizard. `go test -count=1 ./...`, `go vet ./...`, dev binary build и Bash syntax PASS. Избранные race PASS. **НЕТ React production build, настоящего lockfile, E2E, VPS/SSH/Xray/VPN handshake.** Этап 4 `SOURCE_IN_PROGRESS / EXTERNAL_PENDING`, не `ACCEPTED`. Этапы 1–3 остаются `EXTERNAL_PENDING`, SSH hardening из scope 3 не реализован.

**Главные ссылки:** [0.6.1 handoff](release-0.6.1-stage4-continuation.md), [актуальная задача](ai/current-task.md), [чеклист](planning/stage4-checklist.md), [локальные тесты](testing/local-results/stage4-2026-11-04-continuation.md), [внешняя приёмка](testing/stage4-codex-handoff.md).

---

## Историческое состояние архива 0.6.0 и 0.5.1 (не применять как актуальный статус)

### Архив 0.6.0-stage4-source-preview

**2026-11-04:** этап 4 имеет рабочую кодовую реализацию базового контроллера/агента/SSH install/remote VLESS Reality и Go-тесты, но **не завершён полностью**. Статус: `STAGE4_IMPLEMENTATION_PARTIAL / GO_LOCAL_PASS / FRONTEND_BLOCKED / EXTERNAL_PENDING`. Нельзя считать принятым или production ни этап 4, ни этапы 1–3 до их внешней приёмки. [Полный кодовый handoff](release-0.6.0-stage4-source.md), [эксплуатация](ops/stage4-nodes.md), [список открытых работ](planning/stage4-checklist.md), [тесты](testing/local-results/stage4-2026-11-04.md).

| Контур этапа 4 | Что подтверждено | Что не подтверждено |
|---|---|---|
| SQLite revisions/operations, node secret rotation/revoke | Реализовано; Go unit + HTTP/security тесты | Реальный lifecycle/upgrade/rebind/backup перенос узлов |
| Node agent + Unix root-helper | Типизированный apply, rollback, receipt, peer UID; Go tests/race | Поведение systemd и фактический VPN трафик на VM |
| SSH signed install | Код preflight, pinning, Ed25519 bundles, staged install; shell syntax и Go tests | Реальная SSH установка, чистая VM, UFW/custom SSH, внезапный обрыв |
| React servers UI | Исходный TSX и API-контракты | `npm ci`, TypeScript build, Playwright, мобильная доступность |
| Node readiness/publication | `agent` vs `xray` vs `vpn_probe=unknown`, `published=false` | External probe, production публикация endpoint, полное управление узлами |

**Критические незавершённые задачи:** React lockfile/build и E2E, внешний независимый VPN handshake, полноценный alternate CLI installer, safe uninstall/upgrade/rebind, расширенный host preflight/diff, SSH hardening (долг stage3). Без закрытия этих пунктов **этап 4 не закончен**.

---

## Исторический статус предыдущего архива 0.5.1 (сохраняется как baseline)


**Дата: 2026-11-04. Источник истины:** результаты из `docs/testing/local-results/stage3-2026-11-04.md`, код и внешний протокол испытаний, а не скриншоты.

| Подсистема | Реализовано в исходниках | Проверено здесь | Внешняя проверка |
|---|---|---|---|
| Go, SQLite, учётные записи, VPN-профили, QR/подписки | Да | `go test` + `vet` | Реальные пользователи и VPN-трафик |
| React, TypeScript, Vite, desktop/mobile UI | Исходники и реальные API | Синтаксис 16 TS/TSX файлов | `npm ci`, `typecheck`, `build`, Playwright, 320–1440 px |
| VLESS Reality TCP, XHTTP, Hysteria2, WG, AWG 3.1 | Конфиг/управление движками | Контрактные тесты Go | Установка и реальное подключение пяти вариантов |
| 2FA TOTP, CSRF, сессии, CLI reset | Да | Unit и HTTP тесты | HTTPS, восстановление доступа на VPS |
| Лимит файлов журнала панели | Да | Go тесты | Нагрузка/ротация на VPS |
| Изолированный journald ArcVPN + лимит | UI + API + root helper + install unit | Go/Bash | journald namespace, лимит, независимость SSH-логов |
| UFW | UI + API + root helper, только ArcVPN-labelled порты | Go-тесты, negative cases | Реальный UFW, custom SSH port, rollback, отсутствие lockout |
| Зашифрованный экспорт/импорт ArcVPN v3 | Клиенты, VPN, ключи, подписки, реестр нод, лимит логов | Go snapshot/rollback | Полный перенос на новый IP и обновление адресов агентов |
| Периодические encrypted CLI backup | Запись 0600, retention, opt-in timer | Go tests/Bash syntax | Работа timer, storage и восстановление на VPS |
| Подписанные обновления | Ed25519 verify, staging, UI, подписанные бинарники панели и движков, rollback | Go tests + race subset | Реальная смена релиза, отказ и восстановление service |
| HTTPS по IP и certbot renew | Установочный сценарий | Синтаксис Bash; CLI-флаги сверены с docs | Настоящая выдача/продление IP-сертификата |
| Дистанционные ноды | Telemetry-only preview: enrollment, bearer-secret, heartbeat и метрики | Go тесты | **Этап 4 NOT STARTED:** отсутствуют SSH-wizard, удалённая установка VPN, доверенный apply и подтверждение ревизий |
| Каскады, маршруты, автоматический выбор | Нет | Нет | Этап 5 |
| Дополнительные протоколы | Нет | Нет | Этап 6 |

**Зафиксировано в исходном handoff этапа 3:** локальные Go-проверки пройдены, но React-сборка и внешние проверки остаются неподтверждёнными. При дополнительном аудите выявлено отсутствие предусмотренного roadmap SSH hardening, поэтому полнота кода этапа 3 не доказана. **Приёмка этапа 3:** `EXTERNAL_PENDING`. **Production:** нет. Вершину многоузловой инфраструктуры нельзя сохранить как действующую, пока её код не реализован в этапах 4–5. Реестр нод и их токены сохраняются, но URL старого контроллера в конфигурациях установленных агентов автоматически не меняется.

## Аудит 2026-11-04 перед этапом 4

Повторно запущены `go test -count=1 ./...`, `go vet ./...`, `bash -n`, dev `go build` — **PASS**. `npm ci`/React typecheck/build/E2E и реальные VPS не запускались. Исходный ZIP прошёл SHA-256 проверку. Подробности: [аудит](audits/2026-11-04-pre-stage4.md), [локальные результаты](testing/local-results/pre-stage4-2026-11-04.md).

**Дополнительный scope gap:** дорожная карта этапа 3 предполагает SSH hardening из UI, но такого API/UI в исходниках 0.5.1 нет. Нужно завершить либо формально согласовать перенос, до заявления о кодовой полноте stage3.

**Текущая точка разработки:** начать этап 4 с [чеклиста](planning/stage4-checklist.md). Устранить утечку enrollment token через argv и описать trust/rollout до новых возможностей удалённого управления. Проект **не production**; этапы 1–3 остаются `EXTERNAL_PENDING`, этап 4 — `PLANNED / NOT STARTED`. Этот документальный handoff не реализует stage4 функции.
**2026-11-05 — устройства локального Xray:** отдельные credentials/подписки, серверный лимит/CAS, отзыв, UI и backup v7 реализованы; общий ключ после перехода отключён. Физические устройства не аттестуются; remote/UDP devices и live disconnect/overshoot не подтверждены. [Контракт](adr/009-device-credentials.md), [результаты](testing/local-results/stage5-2026-11-05-devices.md).
**2026-11-05 — Hysteria2:** локальный учёт trafficStats, общий пакет Xray/Hysteria, credentials устройств и backup v8 реализованы. Реальный loopback QUIC/TLS v2.13.0 подтвердил upload/download и отказ отозванному ключу после restart. Systemd/procfs на VPS, external ACME/egress и overshoot не приняты. [ADR-010](adr/010-hysteria-traffic-and-devices.md), [проверки](testing/local-results/stage5-2026-11-05-hysteria.md). Следующие источники — WG/AWG и remote nodes.
