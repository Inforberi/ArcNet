# Этап 4: проверка требований на исходниках 0.6.5

Дата: 2026-11-04. Это **проверка реализации в исходниках**, не акт приёмки. Основания: исходный чек-лист `stage4-checklist.md`, конкретные файлы, unit/security tests, реальный вывод `verify-stage4-local.sh --source`. Наличие кода без интеграционной проверки не равно выполненному требованию на VPS. В колонке «Код»:

- **Есть** — явная реализация основного сценария найдена в указанных файлах.
- **Частично** — требование шире проверенного поведения, его нельзя закрывать отметкой `[x]`.
- **Блокер** — обязательный шаг недоступен в данном окружении.
- **Вне этого этапа** — зависимость из более раннего этапа, которую нельзя скрыть.

| ID | Код | Подтверждённая граница и остаток |
|---|---|---|
| S4-00 | Есть | Исходный аудит `docs/audits/2026-11-04-pre-stage4.md`, модель угроз и security regressions. Независимый аудит отсутствует. |
| S4-00a | Вне этого этапа | SSH hardening локальной панели из этапа 3 не заменяется SSH wizard. Остаётся отдельный долг и не помечен закрытым. |
| S4-01 | Блокер | `frontend/package-lock.json` отсутствует; npm отвечает `EAI_AGAIN`. Реальный `npm ci`, typecheck, Vite build и E2E не запущены. |
| S4-02 | Частично | ADR-002 и threat model описывают TLS server authentication + scoped bearer. **Не mTLS**; отдельная security review не пройдена. |
| S4-03 | Есть | Enrollment по stdin, креденшелы в приватном файле, no-follow `nodeagent/agent.go`; `store/node_control.go` защищает AES key. Работоспособность on-VPS не подтверждена. |
| S4-04 | Есть | `nodebootstrap/install.go`, `internal/updater`: pinned Ed25519 release и проверка бинарников после копирования на root-owned путь; реальная совместимость Xray не проверена. |
| S4-10 | Частично | `store/node_control.go`, `nodes.go`, `node_bootstrap.go`, `node_probe.go`: схему и миграции проверяют Go-тесты; не вся исходная расширенная модель полей явно присутствует. |
| S4-11 | Частично | Persistent operation ID, журнал, attempts, terminal status; нет полноценно документированного owner/timeout per operation в DTO и независимой проверки restart сценариев на VPS. |
| S4-12 | Частично | UI/API разделяют agent-online, Xray-state, revision, VPN-probe. Отдельный фактический `route_health` для маршрутизации (этап 5) здесь не реализован. |
| S4-13 | Частично | Типизированные agent poll/ack и административные endpoints существуют; полного versioned JSON schema для всех SSH bootstrap/diff/rotation операций нет. |
| S4-14 | Есть | Монотонная desired ревизия, digest/AES-GCM, operation ID и stale ACK protection в `store/node_control.go`; тесты. |
| S4-15 | Есть | Без независимого подтверждения `published=false`; диагностические ссылки не превращаются автоматически в подписку. |
| S4-16 | Частично | Перепривязка агента реализована, AES-key не переносится в portable backup; совместное восстановление нового контроллера и VPS не испытывалось. |
| S4-20 | Частично | Исходящий HTTPS + bearer, polling, bounded backoff/jitter в `nodeagent/agent.go`, per-node identity. Нет mTLS и нет доказательства on-VPS TLS/retry. |
| S4-21 | Есть | Одноразовый enrollment, hash токена, rotation/revoke; unit и HTTP tests. |
| S4-22 | Есть | `nodeprotocol`, `noderuntime`: фиксированная команда `apply_xray_v1`, digest/revision/protocol; нет произвольного shell RPC у агента. |
| S4-23 | Есть | Раздельные учётные записи агента/Xray, systemd socket root helper с ограниченным API в `nodebootstrap/install.go`; права на реальном Debian/Ubuntu ещё не проверены. |
| S4-24 | Частично | Local stage/validate/atomic apply/receipt/rollback в `noderuntime` и unit tests; отказ питания, systemd и последний рабочий Xray на VPS не проверены. |
| S4-25 | Частично | `nodeagent.Version=0.6.5-stage4`, `nodebootstrap/version.go` запрещает downgrade/другую ветку; нет подтверждённой матрицы реальных Xray versions/configs. |
| S4-26 | Есть в коде | Serial apply, bounded attempts, explicit retry/cancel, TTL+size retention обоих журналов (`node_control.go`, `node_bootstrap.go`), bounded agent backoff. Внешний долгий restart/recovery не протестирован. |
| S4-30 | Частично | Read-only SSH preflight определяет arch, os, systemd, CPU, RAM, disk, sudo, listening TCP port, UFW/nft и HTTPS egress; матрица поддерживаемых версий ОС не подтверждена VM-тестами. |
| S4-31 | Есть | `ResolvePublic` public IP allowlist, pinned SSH Ed25519 host fingerprint, no `InsecureIgnoreHostKey`; поддержаны IPv6 literal и bracketed SCP. Проверка подлинности fingerprint требует OOB-канала. |
| S4-32 | Частично | Server-side persistent job с этапами, UI approval и self-serve install; структурированный deterministic diff и атомарная доставка стадии `probe` на удалённом хосте не полностью подтверждены. |
| S4-33 | Есть в коде | Подписанная установка агента и Xray, root-owned binary hash, rollback в `nodebootstrap/install.go`; выполнение на реальной VPS отсутствует. |
| S4-34 | Есть в коде | CLI `node-install` с заранее подписанным bundle и stdin secret; не является небезопасным `curl | bash`; реальная single-command VM приёмка не пройдена. |
| S4-35 | Частично | SSH/UFW не изменяются установщиком, поэтому нет риска *автоматической* блокировки со стороны этой операции; внешние firewall/connectivity и пользовательский SSH-port не проверены. |
| S4-36 | Частично | detach/purge/revoke/rebind с managed marker, идемпотентными readback; реальные power-loss/recovery и внешний VPN трафик не проверены. |
| S4-40 | Частично | React `Nodes.tsx` содержит SSH wizard/preflight/install/removal/upgrade, серверный journal; отсутствует TS typecheck и Playwright. |
| S4-41 | Есть в коде | `agent`, Xray, probe, revision, draining имеют разные UI поля. `PASS` от grant — только заявленный оператором результат, не attestation. |
| S4-42 | Есть в коде | Table/diagram используют один `nodes.data.items`; UX/a11y E2E не подтверждены. |
| S4-43 | Частично | Admin действия связаны с backend (revoke, drain, apply, retry, cancel, remove, upgrade), но браузерные ошибки/подтверждения не прошли E2E. |
| S4-44 | Блокер | React `npm ci`, typecheck, build, браузер на 320/390/768/1024/1440 недоступны без зависимостей. Синтаксис TSX — не замена. |
| S4-50 | Частично | Go unit/security/HTTP tests пройдены; не все end-to-end сценарии секретов/SSH/TLS подтверждены. |
| S4-51 | Частично | Go fake systemctl, stale ACK, duplicate, no-follow, recovery, SQL migration и race покрыты; реальные disk-full, controller power-loss и SSH interruption не выполнены. |
| S4-52 | Блокер | Go test/vet/race/build, shell/Python, TS syntax — PASS; React production/tests и Xray integration — NOT RUN. |
| S4-53 | Блокер | Нужна подготовленная VM/VPS с snapshot, root/systemd, настоящим Xray и внешним клиентом. |
| S4-54 | Блокер | Нужны две отдельные тестовые VPS и независимый клиент для полного lifecycle и восстановления. |
| S4-55 | Блокер | `LOCAL_PASS` не достигнут без frontend build/E2E и полной локальной traceability, `STAGE4_ACCEPTED` без внешних протоколов невозможен. |

## Не путать с закрытием этапа

Go-кодовая проверка (`SOURCE_CHECKS_PASS`) — это не доказательство работы React или VPN. Для завершения исходного объёма требуется: реальная сборка frontend с зафиксированным lockfile; устранение выявленных TypeScript/E2E дефектов; подтверждение операций на двух изолированных VPS и внешнего Xray/VLESS/REALITY egress. Требования с пометкой «Частично» нельзя автоматически переклассифицировать в «Есть» без тестов и недостающей реализации. **Не переходить к этапу 5 в рамках последовательного процесса, пока stage4 не принят.**
