# Состояние единого этапа 4 — 0.6.5-stage4

**НЕ ЗАВЕРШЁН:** source Go/security verification PASS, но React dependencies/typecheck/build/E2E заблокированы `EAI_AGAIN`, внешние VPS/Xray/VLESS/REALITY тесты не запускались. Полное состояние **каждого ID S4-00…55** зафиксировано в [матрице 0.6.5](stage4-traceability-0.6.5.md); она актуальнее исторических `[x]` ниже, отмечавших только кодовую реализацию. [Тесты](../testing/local-results/stage4-2026-11-04-0.6.5.md). Не выдавать `SOURCE_CHECKS_PASS` за `LOCAL_PASS` или `STAGE4_ACCEPTED`.

---

# Приёмка этапа 4: актуальная редакция для 0.6.4

**Не завершён.** Ниже сохранён исторический scope S4-xx; отметки `[x]` означают только локально проверенную реализацию. Закрыты дополнительные дефекты безопасности: `ReadConfig` TOCTOU, подписанный archive TOCTOU/DoS, exposed private signing key, rollback чужих пользователей, stale receipt ACK, несогласованные HTTPS-origin и shell quoting. Код, тесты и ограничения: [0.6.4](../release-0.6.4-stage4-security-and-acceptance.md).

- [x] Повторный `applied` подтверждает совпадение байтов конфигурации и `systemctl is-active`, иначе FAIL; damaged/symlink receipt ведёт к fail-closed.
- [x] Рестрикция private credentials / signed bundle / signing key через no-follow bounded files; регрессионные тесты.
- [x] Отказ установки не удаляет уже существующие системные аккаунты в account-creation race.
- [x] Единая строгая HTTPS-origin validation; корректное POSIX shell quoting отображаемых CLI-команд.
- [x] `go test`, `go vet`, dev Go build, targeted race критических пакетов/API, shell/Python/TS syntax — PASS.
- [ ] `S4-01/S4-44/S4-52` Настоящий npm lockfile, `npm ci`, `typecheck`, Vite build, Playwright E2E.
- [ ] `S4-53/S4-54/S4-55` Изолированная VPS + настоящий Xray + реальный клиентский handshake и отказные сценарии + протокол приёмки.
- [ ] `S4-00a` Перенос/реализация SSH hardening этапа 3 по отдельному согласованию.

**Критерий окончания единый:** ни частичные source версии, ни кодовый PASS не равны принятию этапа. Не начинать этап 5 до приёмки stage4, если требуется последовательная модель разработки.

## Историческое дополнение к чек-листу 0.6.3

**Дата 2026-11-04.** Статус `SOURCE_IN_PROGRESS / GO_TEST_PASS / FRONTEND_BLOCKED / EXTERNAL_PENDING`. Исторический список требований ниже сохранён. `[x]` означает **только исходники плюс локальные тесты**, не VPS-приёмку. [Подтверждённое](../release-0.6.3-stage4-continuation.md); [тесты](../testing/local-results/stage4-2026-11-04-0.6.3.md).

- [x] `S4-12a` Отдельный `vpn_probe` с состояниями `unknown/pass/fail/stale`, timestamp и привязкой к последней применённой revision/digest; heartbeat никогда не означает data-plane PASS.
- [x] `S4-12b` Независимый Xray VLESS/REALITY TCP+Vision клиент с loopback SOCKS5, без direct fallback, подтверждение HTTPS 204 по TLS, timeout и чистка временного файла.
- [x] `S4-12c` Защищённый grant на 5 минут (hash в SQLite, replay guard, CSRF/admin issue, bearer report, ревизия/digest, конкурентные отчёты, reset при drain/deploy/upgrade/remove; 30 минут freshness); никаких auto-publication.
- [x] `S4-41a` UI отображает фактический последний принятый отчёт с датой, статусом и временным grant; устаревшие проверки видны как `stale`.
- [x] `S4-50a` Тесты ссылки, SOCKS, фальшивого Xray, SQLite migration 0.6.2, API auth/CSRF/replay и защищённого файла CLI; Go vet и race ключевых пакетов.
- [ ] `S4-01` **P0** Настоящий npm lockfile, `npm ci`, TypeScript typecheck, production Vite, Playwright E2E; здесь `EAI_AGAIN`.
- [ ] `S4-53` **P0** Независимая VM с Xray и реальным handshake/egress, evidence/expiry и сетевые ошибки; тестировать отрицательные сценарии.
- [ ] `S4-54` **P0** Подписанный SSH install/update/detach/purge/rebind, recovery after power/SSH loss, external client и firewall на реальных VPS.
- [ ] `S4-55` **P0** Полный программный и внешний acceptance; остальная исходная матрица S4-xx ниже остаётся открытой до независимых доказательств. Probe с bearer grant не является доказательством физической независимости клиента.

---

# Этап 4 — пошаговый план разработки удалённых нод

**Статус на 2026-11-04 (0.6.2): `SOURCE_IN_PROGRESS / GO TEST PASS / FRONTEND BLOCKED / EXTERNAL PENDING`.** Основная реализация перенесена с `0.5.1-stage3-code-handoff` через `0.6.1-stage4`; текущая точка — `0.6.2-stage4`. Отчёт: [release 0.6.2](../release-0.6.2-stage4-continuation.md). Чекбокс `[x]` означает только **код реализован и покрыт указанными локальными Go-тестами**, не внешнюю приёмку. Полный этап 4 ещё не завершён. Исторические требования ниже сохранены как scope. **Относительно предыдущих этапов:** можно начать программирование при `EXTERNAL_PENDING` этапов 1–3, но это не заменяет их внешнюю приёмку. Источник требований: [nodes-and-installer](../features/nodes-and-installer.md), [server-security](../features/server-security.md), [ADR-002](../adr/002-node-reconciliation.md), [topology-and-table](../design/topology-and-table.md), [acceptance](acceptance.md).

**Продолжение 0.6.2:** root-owned checksum при install/upgrade, консервативный SSH readback после потери ответа, egress HTTPS/UFW read-only preflight, `drain` (только запрет **новых диагностических ссылок**, НЕ реальный drain действующих клиентов) и миграция SQLite реализованы с Go-тестами. Browser E2E script исправлен, но не запущен. [Факты](../release-0.6.2-stage4-continuation.md). Незакрытые P0: настоящий React build, проверка VPS/systemd/handshake, независимый VPN-probe. НЕ ставить `[x]` для S4-30/S4-43/S4-52/S4-55.

**Обновление после продолжения:** signed локальный CLI installer, managed-only detach/purge, SSH uninstall, rebind и подписанный upgrade реализованы в исходниках, Go unit/test проверены. **Это не внешняя приёмка:** React production/VPS/systemd/Xray/handshake НЕ проверены. Пункты `S4-25/30/32/34/36/40/43` остаются открытыми до выполнения всех требований и реального стенда. Состояние конкретных файлов перечислено в [0.6.1](../release-0.6.1-stage4-continuation.md).

**Общая цель этапа:** безопасно добавить чистый второй VPS через UI или одноразовую команду; установить и зарегистрировать агент + VPN-движок; применить желаемую конфигурацию с подтверждённой ревизией и автоматическим возвратом на last-good при ошибке; показать раздельные статусы связи, сервиса и VPN. Без реализации каскадов этапа 5.

**Строгое правило отчёта:** каждый чекбокс отмечается только после реализации и доказанной проверки; новые файлы в таблицах ниже — **планируемые**, не существующие. В конце блока обновлять `docs/ai/current-task.md`, `docs/implementation-status.md`, `docs/ai/change-log.md`, `docs/ai/index.md`; тестовые команды и вывод сохранять в `docs/testing/local-results/`.

## 0. Блокирующие основания перед безопасным rollout

- [x] `S4-00` Прочитать audit [2026-11-04-pre-stage4.md](../audits/2026-11-04-pre-stage4.md); внести в работу найденные P0/P1, не повторять их в новом bootstrap.
- [ ] `S4-00a` Отдельно урегулировать незавершённую часть этапа 3: проектное требование на SSH hardening из `docs/planning/roadmap.md` не найдено в API/UI. Либо выполнить как stage3 debt, либо письменно зафиксировать перенос в stage4 до объявления этапа 3 code-complete. SSH bootstrap wizard этапа 4 не заменяет управление SSH-настройками локального VPS.
- [ ] `S4-01` Получить воспроизводимую сборку React: добавить **реальный**, созданный npm lockfile; выполнить чистый `npm ci`, `typecheck`, `build` и занести результат. Нельзя сгенерировать lockfile вручную.
- [ ] `S4-02` Утвердить контракт управления нодой и trust boundary: типы операций, аутентификация, угрозы SSRF/agent compromise, ротация/revocation, хранение и доставка секретов, версии payload и обратная совместимость. Обновить ADR, если выбран новый протокол связи; не объявлять заранее mTLS реализованным.
- [x] `S4-03` Перепроектировать enrollment/bootstrap, чтобы токен, SSH-пароль, приватный ключ и конфиги с секретами **не попадали** в argv, shell history, env долгоживущего процесса, логи или SQLite как открытый текст; покрыть тестами утечки. Текущий `--token` — ограничение preview, не образец для production.
- [x] `S4-04` Определить независимый trust для бинарника и обновлений ноды (release signatures/pinned public keys, architecture and digest), fail-closed при отсутствии ключа или плохой подписи; не считать HTTPS+хеш с того же сервера подписью.

## 1. Контракт данных, API и управление состоянием (backend)

- [ ] `S4-10` Расширить `managed_nodes` миграциями SQLite: identity, desired/applied revision, last-good digest, lifecycle/status/timestamps, capabilities, version, failure reason; сохранить старые данные и backup v3, протестировать upgrade и downgrade policy.
- [ ] `S4-11` Ввести модель `NodeDeployment` и `NodeOperation` (идемпотентный operation ID, owner, этап, timeout, retry budget, journal, timestamps, redacted error); отсутствие подтверждения не переводит операцию в success.
- [ ] `S4-12` Разделить состояния `agent_connectivity`, `xray_service`, `vpn_probe`, `route_health`, `revision_status`. У `vpn_probe` может быть `unknown`, особенно без внешнего тестера. Не использовать heartbeat как доказательство готовности VPN.
- [ ] `S4-13` Определить строгие JSON-контракты и версионирование для bootstrap/preflight/plan/apply/ack/status/rotate/revoke; настроить проверку прав/CSRF/admin на контроллере. Текущие `GET/POST /api/nodes*` сохранить совместимыми, новые endpoints добавлять после тестов.
- [x] `S4-14` Реализовать atomic desired revision, сверку с last-good и monotonic applied revision, защиту от stale/duplicate/out-of-order ACK; тесты restart/timeout/retry, rollback и конкурентных обновлений.
- [x] `S4-15` Запретить публикацию непроверенного endpoint в подписку; пока этап 5 не реализован, удалённый inbound может быть помечен только в пределах ограниченного прямого сценария после внешней проверки. Разделить `installed`, `agent_online`, `vpn_ready`, `published`.
- [ ] `S4-16` Явная политика для backup/import при смене контроллера, включая перепривязку агентов: не считать перенос token hashes достаточным для восстановления управления.

**Существующие точки интеграции:** `internal/store/{store.go,nodes.go,portable_nodes.go}`, `internal/httpapi/{httpapi.go,nodes.go}`, `internal/app`, `internal/backup`, `internal/protocols`, `internal/engines`. Новые пакеты создавать только при наличии реальной ответственности, а не ради абстракции.

## 2. Защищённый канал и реальная работа агента

- [ ] `S4-20` Заменить telemetry-only цикл в `internal/nodeagent/agent.go` версионированным, взаимно аутентифицированным каналом controller↔agent (агент инициирует исходящее соединение; сертификаты и/или иной доказанный эквивалент выбираются в ADR), с timeout/backoff/jitter и bounded workloads.
- [x] `S4-21` Индивидуальная identity ноды, короткий срок enrollment, однократное использование, ротация credentials без остановки уже применённого VPN; отзыв одной ноды не затрагивает другие.
- [x] `S4-22` Агент принимает **только заранее определённые типизированные операции** с проверкой версии, scope, payload hash, revision и подписи/identity. Не делать shell RPC, передачу произвольных путей или системных команд.
- [x] `S4-23` Разделить непривилегированный агент и минимальный root-helper для controlled installation/config/restart; helper должен принимать только ограниченные ресурсы ArcVPN и иметь независимые проверки. Определить systemd hardening и ownership.
- [x] `S4-24` На ноде: stage → validate config → backup last-good → atomic install → controlled service reload/restart → local checks → ACK. При любой ошибке восстановить previous; зафиксировать подтверждённый rollback; после обрыва контроллера работающий VPN продолжает обслуживать трафик.
- [ ] `S4-25` Описать совместимость версий агента, движков и конфигов. Исправить hard-coded `Version = "0.3.0-preview"`; запретить некорректные automatic upgrades/downgrades.
- [ ] `S4-26` Ограниченная очередь и журнал операций: отмена задачи, retry/backoff, serial apply на ноду, дедупликация, bounded retention; обновление панели/агента посреди операции не теряет состояние.

**Запреты:** доверять только `last_seen`, запускать произвольные пользовательские команды, перезаписывать чужие конфиги, логировать секреты, обновлять root-компонент без проверяемой подписи или rollback.

## 3. Bootstrap VPS — оба режима установки

- [ ] `S4-30` Реализовать **read-only preflight** по выбранному SSH endpoint: версия ОС/архитектура, CPU/RAM/диск, systemd, sudo, порты, сетевой доступ, конфликт процессов, поддержка пакетов и требуемых движков. Только подтверждённая матрица совместимости.
- [x] `S4-31` Безопасный SSH host-key pinning: показать fingerprint и получить осознанное подтверждение, отклонять MITM и неожиданные изменения; не использовать `InsecureIgnoreHostKey`. SSRF/IP allowlist для URL/SSH, отдельная политика на private/link-local/cloud-metadata addresses.
- [ ] `S4-32` Server-side install operation с шагами `preflight → plan(diff) → approval → stage → verify → install → enroll → apply → probe`; уникальный ID и журнал. Закрытие вкладки не отменяет/не дублирует установку.
- [x] `S4-33` Установка *и* удалённого Xray, *и* агента на второй VPS. Не выполнять `curl | sudo bash` без проверки происхождения, версии и целостности, при ошибке не оставлять частично управляемую ноду как healthy.
- [ ] `S4-34` Альтернативная **одноразовая команда** без открытых токенов в истории и argv; она должна иметь тот же verified bootstrap, идемпотентность, timeout и rollback. После применения не оставлять bootstrap-полномочия.
- [ ] `S4-35` Безопасная работа с SSH/UFW: никогда автоматически не отключать существующий SSH, сохранять custom port, минимальные managed firewall rules; rollback по таймеру и независимый probe на отдельной VM. Не трогать чужие службы.
- [ ] `S4-36` Удаление/замена/перерегистрация: варианты revoke only, uninstall only managed files, оставить standalone VPN; подтверждение на опасные изменения и полный аудит.

**Текущий `web/node-install.sh` — preview:** сейчас создаёт user, ставит агента мониторинга и unit; не считать его проверенным implementation для `S4-33`.

## 4. React UI: реальный wizard и схема/таблица

- [ ] `S4-40` Страница `frontend/src/pages/Nodes.tsx`: wizard адрес/SSH порт/учётные данные/подтверждение host key, preflight, автоматический безопасный план, diff, запуск и server-side progress, обработка ошибок/повтор. Секреты не сохраняются в localStorage и не выводятся в UI после завершения операции.
- [x] `S4-41` Разделить статус связи, Xray service, фактический VPN probe и published revision; отображать stale/unknown и причину деградации, а не «online = работает VPN».
- [ ] `S4-42` Схема и таблица — две проекции одного typed API state: ноды, доступные логические endpoint, активная выборка, ошибки, фильтры и timestamps. Визуальный граф не выдумывает каскад до этапа 5.
- [ ] `S4-43` Операции над нодой: inspect/status, rotation, revoke, retry failed rollout, drain/unpublish, безопасная деинсталляция; каждый action соответствует backend, dead-end кнопки не допускаются.
- [ ] `S4-44` Responsive keyboard/a11y/light/dark, E2E для 320/390/768/1024/1440 px; все реальные API ошибки показаны пользователю с кодом операции без секретов.

## 5. Тесты и приёмка

- [ ] `S4-50` Unit + HTTP/security tests: token expiration/single use/rotation/revoke, CSRF/auth, SSRF, host-key mismatch, malformed payload, replay/duplicate ACK, idempotency, migration, backup + rollback, отсутствие секретов в логах/argv.
- [ ] `S4-51` Integration/fault injection: агент offline / delayed ACK / restart / controller restart / corrupted config / disk full / unit fails / quorum отсутствует / concurrent deploy. Последняя работоспособная конфигурация сохраняется; ошибочный deployment не помечается healthy.
- [ ] `S4-52` Локальные сборки: `go test -count=1 ./...`, `go vet ./...`, релевантный `go test -race`, `bash -n`, `npm ci`, `npm run typecheck`, `npm run build`, React E2E; фиксировать реально выполнимое и отдельный `NOT RUN`.
- [ ] `S4-53` Отдельный VPS с rescue/snapshot: SSH pinning, clean install, systemd/UFW, реальный Xray, подключение VPN с внешнего клиента, сбой применения и восстановление, отключение контроллера без остановки VPN.
- [ ] `S4-54` Два VPS: проверка второго узла из UI, update/revoke/reinstall, agent upgrade/reboot, TTL/staleness, замена контроллера, публикация только после внешнего probe. Результаты — [handoff](../testing/stage4-codex-handoff.md) и реальные файлы в `docs/testing/external-results/`.
- [ ] `S4-55` Gate `LOCAL_PASS` только после полного написания кода и доступных тестов; `ACCEPTED` только при документированном PASS AC-08/09/15/17/20/21/22/25/27/29/30/39 и всех относящихся к этапу 4 сценариев. Проверить отсутствие ложных утверждений в README/AGENTS/status.

## Последовательность выполнения и приоритеты

1. **4A — спецификация безопасности и готовность сборки:** `S4-00`, `S4-00a`, `S4-01..04`. Самая ранняя задача — убрать передачу токенов через argv; архитектура доверия до новых RPC.
2. **4B — контракт данных и канал:** `S4-10..16`, `S4-20..26`; локальные fault-injection/security тесты прежде удалённого rollout.
3. **4C — установщик и ноды:** `S4-30..36`; проверка всех привилегированных шагов и отката до UI-автоматизации.
4. **4D — UI и сценарии:** `S4-40..44`; затем `S4-50..55`, полный handoff на тестовый VPS и исправление найденных дефектов.

**Вне этапа 4:** автоматические каскады A→B/B→A, route graph/failover, probe из разных клиентских сетей и гарантии сохранения текущих TCP-сессий (этап 5); AnyTLS/Trojan/TUIC/SS2022 (этап 6). Это не должно задерживать простой работоспособный прямой VPN через удалённую ноду.
