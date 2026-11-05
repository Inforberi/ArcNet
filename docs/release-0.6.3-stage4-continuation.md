# ArcVPN 0.6.3-stage4 — полный source handoff, продолжение этапа 4

Дата: **2026-11-04**. Основа: `arcvpn-0.6.2-stage4-source-full.zip`. Этап 4 **не завершён**: `STAGE4_SOURCE_IN_PROGRESS / GO_TEST_PASS / REACT_BUILD_BLOCKED / EXTERNAL_PENDING`.

## Изменения и назначение

Проблема версии 0.6.2: `agent_online`, `xray_active` и ACK применённой ревизии показывали только control-plane; отдельная независимая проверка VPN data-plane отсутствовала и `vpn_probe` всегда оставался `unknown`.

В 0.6.3 реализованы:

1. `internal/nodeprobe/probe.go`, `socks.go`: отдельный клиентский probe (на отдельной Linux-машине), строго VLESS/REALITY TCP + Vision, заданный публичный IPv4/IPv6 ноды, независимый Xray без `freedom`-outbound, localhost SOCKS5 с рандомной аутентификацией, успешный HTTPS `GET https://www.gstatic.com/generate_204` с **204 и проверкой TLS**. Жёсткие тайм-ауты, временный каталог режима 0700 и конфигурация 0600; ошибки не раскрывают UUID/link/ключи. Нет fallback по прямому соединению.
2. `cmd/arcvpn/node_probe.go`: CLI `arcvpn node-probe --link-file ... --xray ...`; файлы secrets должны быть регулярными 0600, symlink запрещён; при отчетности есть `--report-token-stdin` или защищённый файл; HTTPS origin контроллера валидируется с проверкой CA и без redirect. Выход — JSON без приватных параметров.
3. `internal/store/node_probe.go`: новая таблица `node_probe_grants`, additive миграции `node_control`, grant с TTL 5 минут и SHA-256 hash-токена в БД, атомарный одноразовый `CompleteProbe` на точную применённую ревизию/digest. `vpn_probe` принимает `unknown/pass/fail/stale`, результат старее 30 минут становится stale. Изменение конфигурации, `drain`, удаление и обновление инвалидируют предыдущие результаты/допуски. При `pass` **не выполняется auto publish**.
4. `internal/httpapi/node_probe.go` и `Routes`: admin+CSRF `POST /api/nodes/{id}/probe/grant`, HTTPS bearer `POST /api/nodes/probe/report` от внешнего runner. Обычные session cookies не дают права отчитываться без одноразового допуска. Отчёт привязан к node/revision/digest, повтор/конкуренция отвергаются.
5. `frontend/src/pages/Nodes.tsx`: состояние probe в таблице и схеме, выдача ограниченного по времени допуска, инструкция для отдельного тестера, предупреждение об ограниченности доверия к отчёту. Операции продолжения 0.6.2 сохранены.
6. Unit/security tests `internal/nodeprobe/*_test.go`, `internal/store/node_probe_test.go`, `internal/httpapi/node_probe_test.go`, `cmd/arcvpn/node_probe_test.go`, включая false ACK, replay, drain, миграцию 0.6.2, race и отсутствие автоматической публикации.

## Важное ограничение модели доверия

Получение подлинного one-time grant и отправка `pass` доказывают **авторизацию отправителя**, но сами по себе **не доказывают ни геолокацию, ни факт выполнения клиента**. Только доверенный внешний runner/оператор в отдельной сети и его очищенные логи дают операционное доказательство. Это не криптографическая удалённая аттестация. Пока независимый запуск не сделан, никакой реальной доступности VPN не подтверждено.

**Условия PASS внешней пробы:** реальный клиент Xray запускается, его конфиг проходит `run -test`, соединение проходит через единственный VLESS/REALITY outbound к публичному IP, HTTPS-запрос по SOCKS5 завершается 204. Успех одного URL не гарантирует работу другого назначения, UDP, IPv6, всех провайдеров и долгосрочную доступность. При завершении Xray до прослушивания, неверном SNI/key, тайм-ауте или HTTPS failure возвращается `fail`.

## Фактически выполненные проверки

См. [подробный протокол](testing/local-results/stage4-2026-11-04-0.6.3.md). `go test -count=1 ./...`, `go vet ./...`, отдельная Go CLI сборка, Bash-syntax, TypeScript **syntax** и race ключевых пакетов и отдельный race-тест нового HTTP API — PASS. Общий `go test -race -count=1 ./internal/httpapi` превысил тайм-аут — **TIMEOUT**, не PASS. `npm install --package-lock-only` — **BLOCKED `EAI_AGAIN`**; lockfile не создавали вручную. `npm ci`, `npm run typecheck`, Vite production build, React browser E2E, VPS/systemd, реальный Xray и VPN handshake — **NOT RUN**.

## Что не готово / следующий шаг

- Реальная React production сборка + Playwright, исправление фактических TS/UX ошибок.
- Независимый runner на тестовой машине с поддерживаемой версией Xray; проверить корректность генератора новых JSON полей на реальном Xray и VLESS/REALITY handshake; сохранять очищенные логи и сроки/результаты в `docs/testing/external-results/`.
- Весь оставшийся внешний acceptance этапа 4: signed install/upgrade/uninstall/rebind на VPS, проверка SSH/UFW, failure injection, rollback и независимый сетевой клиент.
- Не присваивать `LOCAL_PASS` до покрытия **всей** требуемой кодовой части и доступных локальных проверок, `ACCEPTED` — только по строгой матрице испытаний. Отдельно открыт недоделанный stage3 SSH-hardening UI.

**Следующий вход для ИИ:** [current task](ai/current-task.md), [stage4 checklist](planning/stage4-checklist.md), [Codex handoff](testing/stage4-codex-handoff.md), [runbook](ops/stage4-nodes.md). Исторические документы 0.6.0–0.6.2 оставлены без изменения фактов.
