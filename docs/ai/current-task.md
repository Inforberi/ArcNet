> **Решение по объёму от 2026-11-05:** набор VPN-протоколов заморожен на VLESS/REALITY TCP, VLESS/REALITY XHTTP, Hysteria2, WireGuard и AmneziaWG 3.1. Этап 6 (AnyTLS, Trojan, TUIC, Shadowsocks 2022, экспертные overrides) **отложен**, не принят и не является следующей задачей Codex. Приоритет — завершение/внешняя проверка уже заложенного функционала этапов 4–5. Подробно: [scope freeze](../planning/protocol-scope-freeze-2026-11-05.md).
# Текущая работа: ArcVPN 0.7.0-stage5-foundation

**Активная итерация 2026-11-05:** пользователь одобрил [UX/автоматизацию](../planning/ux-and-safe-automation-2026-11-05.md), включая одну команду установки, домен по желанию, IP TLS shortlived и ограниченные журналы. Тестового стенда нет; явно разрешена локальная реализация. В рабочем дереве начаты профили маршрутизации/API/UI/backup v5 и ограниченные SNI/TLS-пробы с агентом. Полный новый набор ещё НЕ проверен и не принят; предыдущий PASS ниже относится к предыдущему снимку. Последний targeted `go test ./internal/store ./internal/targetcheck ./internal/routingprofile ./internal/app` — exit 0, но в двух новых доменных пакетах ещё нет тестов. Серверная установка, автоматический выбор/применение SNI и реальный VPN этим не подтверждены.

**Дата:** 2026-11-05. **Режим:** пользователь разрешил исправления целостности маршрутов, backup и интерактивный UI; разрешил коммит/отправку в `Inforberi/ArcNet`. Это не отменяет незакрытый статус этапа 4.

Статус: `STAGE5_DRAFT_UI_IMPLEMENTED / GO_SOURCE_PASS / FRONTEND_BUILD_PASS / ROUTING_UI_E2E_PASS / XRAY_CASCADE_NOT_IMPLEMENTED / STAGE4_EXTERNAL_PENDING`.

Продолжение: [редактор стрелок, целостность нод и backup v4](../release-0.7.0-stage5-routing-ui.md), [точные команды и ограничения](../testing/local-results/stage5-2026-11-05-routing-ui.md). Есть настоящий npm lockfile, TypeScript/Vite build и browser UI/API проверки; живой VPN не проверен. SNI/Geo и параллельная балансировка не добавлялись.

**Новый план по запросу пользователя:** [UX, SNI-каталог с выбором по проверенной TCP/TLS задержке, безопасность, ноды из UI и Geo](../planning/ux-and-safe-automation-2026-11-05.md). План дополнен моделью угроз оператора/DPI и VPS-хостера, автоматическим hardening с проверкой доступа/rollback и включаемой пользовательской маршрутизацией с собственными профилями/правилами по аналогии с Happ (вместо меню RU-шаблонов). Это план, не реализация: runtime/API/БД не изменены, нового VPN PASS нет. Структурированные расширенные настройки текущих возможностей допустимы в плане; stage6 и произвольные overrides остаются отложены.

Уже создано: `internal/routes/{policy,simulate,diff}.go`, `internal/store/routes.go`, `internal/httpapi/routes.go`, API и React UI черновиков маршрутов. SQL миграция, проверки циклов, optimistic concurrency, симуляция failover без доступа к live VPN.

**Следующая задача Codex:** [план и VPS-критерии](../testing/stage5-codex-handoff.md), [чек-лист](../planning/stage5-checklist.md), [точный реализованный объём](../release-0.7.0-stage5-foundation.md). Source tests: [протокол](../testing/local-results/stage5-2026-11-05-foundation.md). Нельзя включать маршруты до межнодовых credentials, staged ACK, probe и separate publish.

---

## Исторический статус этапа 4

Frontend-блокер ниже относится к прежнему снимку. Текущий UI build и новый routing E2E проверены; полная VPS/SSH/systemd/egress приёмка всё ещё отсутствует.

# ArcVPN — текущая задача: единая приёмка этапа 4 (0.6.5-stage4)

**Дата:** 2026-11-04. **Готовность:** `SOURCE_CHECKS_PASS / FRONTEND_BLOCKED / VPS_PENDING / STAGE4_NOT_ACCEPTED`. Этап **не завершён целиком**. Исторические 0.6.0–0.6.4 не являются текущими исходниками.

## Выполненные изменения и доказательства

- Security: no-follow защищённое чтение `node-control.key` (включая отказ на symlink/FIFO/0644), тест `TestNodeEncryptionKeyRejectsSymlinksAndNonRegularFiles`.
- Reliability: TTL/count retention `node_operations` и SSH lifecycle journal с сохранением неизменного managed install ID; тесты `TestNodeOperationJournalPrunesOldTerminalRowsButRetainsCurrent` и `TestSSHJournalRetentionPreservesActiveInstallMarker`.
- SSH: публичные IPv6 literals проходят IP/SSRF policy, SCP использует bracketed remote IPv6; Go-тесты.
- Agent: bounded exponential backoff/jitter (20s–5m), отсутствие наложения циклов, reset после успешной связи; Go-тест.
- Supply chain: CI теперь **блокирует** релиз при отсутствии reviewed npm lockfile; никакого CI-only dependency resolution.
- `scripts/verify-stage4-local.sh --source` — **exit 0 / SOURCE_CHECKS_PASS** (Go tests/vet/targeted race/build, Bash/Python). 17 TS/TSX syntax parse — PASS, **не TypeScript typecheck**.

Полный diff и ограничения: [релиз 0.6.5](../release-0.6.5-stage4-source-audit.md), [протокол тестов](../testing/local-results/stage4-2026-11-04-0.6.5.md), [полная матрица S4](../planning/stage4-traceability-0.6.5.md).

## Не выполнено / блокеры

1. Реальный `frontend/package-lock.json` отсутствует. `npm install --package-lock-only ...` вернул `EAI_AGAIN`; нужно получить проверенный lockfile на сетевой машине, затем запустить `npm ci`, `typecheck`, Vite production build, Go embedded build и Playwright 320/390/768/1024/1440. Это не следует выдавать за «готовый frontend».
2. Реальные systemd/SSH/UFW/Xray install/upgrade/detach/purge/rebind на двух отдельных тестовых VPS, recovery при отказах и подтверждение VPN egress через независимый VLESS/REALITY клиент **не выполнялись**.
3. Часть исходных пунктов S4 шире проверенного кода (полная версия JSON контрактов, diff, matrix совместимости ОС/Xray, state/recovery). Чётко отмечено в [матрице](../planning/stage4-traceability-0.6.5.md); не закрывать эти пункты без подтверждения.
4. Отдельный долг этапа 3 — SSH hardening локального контроллера; SSH-установщик этапа 4 его не заменяет.

## Следующее действие / критерий единой сдачи

Передать полный source ZIP и этот набор инструкций в окружение с доступом к npm, реальным Debian/Ubuntu VPS, проверенным Xray и независимым клиентом. Исправить обнаруженные там ошибки в **этом же этапе 4**. Только при реальном `LOCAL_PASS` и `STAGE4_ACCEPTED` перейти к этапу 5. Полный внешней тестовый план — [handoff](../testing/stage4-codex-handoff.md). Никаких выдуманных результатов/секретов в документах.
