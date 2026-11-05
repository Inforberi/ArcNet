# ArcVPN 0.6.0 — этап 4: исходники, проверки и открытые ограничения

Дата фиксации: **2026-11-04**. Состояние: **`STAGE4_IMPLEMENTATION_PARTIAL / GO_LOCAL_PASS / FRONTEND_BLOCKED / EXTERNAL_PENDING`**. Это handoff полного исходного дерева, **не** принятый production-релиз и **не** полностью закрытый этап 4. Этапы 1–3 также ожидают внешнюю приёмку.

## Реально добавлено в исходники

- Backend SQLite: `node_control`, `node_operations`, `node_enrollment_meta`, `node_profiles`, `node_bootstrap_operations`; индивидуальные hash credentials и шифрование desired Xray JSON / Reality-ключей AES-GCM локальным секретом 0600, миграция с baseline 0.5.1.
- Контроллер: `deploy`, typed poll/ack, desired/applied/last-good ревизии, stale/duplicate ACK защита, cancel, Retry с **тем же operation ID**, восемь попыток доставки, двухфазная ротация и выборочный revoke. Публикация endpoint в общие подписки запрещена без независимого external VPN probe.
- Агент: исходящий HTTPS polling/heartbeat, регистрация через stdin, локальный конфиг 0600, ротация секрета с восстановлением после рестарта и bounded timeout. Нет shell RPC.
- Privileged helper: закрытый тип `apply_xray_v1`, Unix socket с проверкой peer UID, allowlist Xray JSON, digest, Xray `run -test`, атомарный apply, restart/is-active, last-good rollback, root-only receipt.
- SSH: public IP policy/защита от SSRF, Ed25519 SSH fingerprint с обязательным внешним подтверждением, строгий `known_hosts`, ключ во временном защищённом файле, DNS resolution перед SSH, read-only preflight, постоянный журнал установки, подписанный Ed25519 ZIP с Xray и агентом, отсутствие токена в аргументах. Два параллельных установщика максимум. Ошибки не помечаются успешными.
- Remote install script: отдельные пользователи агента/Xray, systemd socket/service, `CAP_NET_BIND_SERVICE` для непривилегированного listener 443, root-owned receipt, проверка отсутствия конфликтующих каталогов и unit, компенсация созданных файлов/units при ошибке. Сторонние `sshd`, UFW и Xray сервисы не изменяются.
- React: страница «Серверы» с таблицей и схемой из одного API, SSH wizard, preflight/plan, журнал установки, ревизии, повтор/отмена/отзыв, тестовые прямые VLESS-ссылки. Присутствуют исходники TSX, но production React не собран.
- Старый опасный `/node-install.sh` отключён. Скрипт создания signed node release: `scripts/build-stage4-node-bundle.sh`.

## Исправлено при дополнительной проверке

1. SSH remote command arguments теперь POSIX shell-quoted; исключена подстановка метасимволов через значения параметров.
2. Xray systemd получил необходимую `CAP_NET_BIND_SERVICE` при запуске не от root.
3. Корневой helper хранит receipt в root-owned `/var/lib/arcvpn-node-root`, изолированном от пользовательской директории агента.
4. Неуспешная установка удаляет созданные управляемые unit/папки/аккаунты в пределах консервативной компенсации; существующие объекты ArcVPN блокируют установку до изменения ОС.
5. Конфликтный Retry после потерянного ACK устранён: retry сохраняет тот же operation ID, digest и revision. Добавлена ограниченная доставка и тест на исчерпание бюджета.
6. Обновление терминального состояния SSH-операции при timeout выполняется через новый краткоживущий context, а не отменённый контекст SSH worker.

## Локальные тесты

`go test -count=1 ./...`, `go vet ./...`, `go test -race -count=1 ./internal/store ./internal/nodeagent ./internal/noderuntime ./internal/nodebootstrap`, `bash -n install.sh scripts/*.sh deploy/*.sh web/node-install.sh` — **PASS** (Linux amd64, Go 1.23.2). Пакет `internal/httpapi` в обычном Go-прогоне PASS; полный race HTTP API **не выполнялся**. Тесты дополнительно проверяют signed release (включая неверный Ed25519 trust anchor), shell quoting, installer script syntax, rev/ACK, root-helper rollback, auth/CSRF, racy concurrent deploy. Полный лог: `docs/testing/local-results/stage4-2026-11-04.md`.

`npm install --package-lock-only --ignore-scripts ...` — **BLOCKED** (`EAI_AGAIN` при запросе `registry.npmjs.org`). `npm ci`, `npm run typecheck`, `npm run build`, React E2E — **NOT RUN**; настоящего `frontend/package-lock.json` нет. Реальный systemd, UFW, SSH/host-key, Xray handshake, VM/VPS, обновление, перенос контроллера — **NOT RUN**.

## Что ещё нужно до завершения этапа 4

- **P0:** получить настоящий npm lockfile/React build и E2E на сетевой сборочной машине; полноценная production release-сборка с закреплённой версией Xray.
- **P0:** чистая VM с реальным SSH/systemd/Xray и независимый VPN-клиент; проверить active 443, console recovery, падения/откаты, ACL файлов и перенос подписей. Без этого `LOCAL_PASS` для полного этапа недопустим.
- **P1 код:** полностью автоматический альтернативный однократный CLI bootstrap `S4-34`; безопасное удалённое удаление managed resources / upgrade/rebind; реальный lifecycle server-side (drain/unpublish/probe), а не только revoke; расширить read-only preflight/plan до сети/пакетов/расширенного diff.
- **P1 код:** фильтры/выбор и responsiveness E2E UI; ограниченные upgrades/версии; обработка interrupted-install без ручной provider console при безопасном восстановлении; операционный граф и журналы.
- **P1 security:** независимый review threat model, TLS server identity/credential lifecycle, host-key/OOB проверка, scanner/private network policy, root-helper ownership и restart/rollback на VM.
- **Долг:** первоначальные этапы 1–3 `EXTERNAL_PENDING`; SSH hardening из требований этапа 3 по-прежнему не реализован в GUI и не должен считаться закрытым.

**Следующее действие:** `docs/testing/stage4-codex-handoff.md` и `docs/planning/stage4-checklist.md`. Наличие Go-тестов и переданного архива означает **частичный локальный код**, а не выполнение всего этапа 4.
