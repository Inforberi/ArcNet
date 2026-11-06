# Постоянный журнал каскада — 2026-11-06

**JOURNAL_LOCAL_PASS / ADMIN_UI_NOT_CONNECTED / EGRESS_PUBLICATION_NOT_IMPLEMENTED / VPS_NOT_RUN.** [ADR-016](../../adr/016-persistent-cascade-deployment.md). Старый A2 binary/prerelease не пересобирался.

## Фактически выполнено

Существующий Linux arm64 `arcvpn-local-test:20261105`, Go1.26.6, network none/native exec tmpfs, SQLite C library. Нет новых npm/Go dependencies или фоновых служб.

- Начальные targeted compile/node/helper checks `go test ./internal/store ./internal/noderuntime ./internal/nodeagent ./internal/app -run '^TestCascade|^TestNodeDeployment|^TestApplySuccess'` — PASS.
- Новый `go test -race -v ./internal/store -run '^TestCascadeJournal'` выявил обход через отдельный legacy `ReplaceState`: он не проходил через full-state guard. Добавлена блокировка до DELETE; защита не ослаблялась.
- После исправления: `go test -race ./internal/store ./internal/noderuntime ./internal/nodeagent ./internal/app` — **exit0**; store1.318s, noderuntime1.042s, nodeagent1.154s, app6.194s. Старые direct/device/usage/restore/helper/agent/app tests входят в эти пакеты.
- После добавления обработки withdrawn capability и её regression: `go test -race ./internal/store -run '^TestCascadeJournal'` — **exit0**,1.078s. Остальные три пакета после успешного общего прогона не менялись.
- `go test ./cmd/arcvpn ./internal/httpapi -run '^TestCascadeBusy'` — **exit0**: основной CLI скомпилирован, целевой HTTP409 test PASS. Это не полный HTTP suite или запуск сервера.
- `gofmt` / `git diff --check` — PASS. Frontend/E2E/release матрица не запускались: UI не изменялся.

## Покрытие

Реальная SQLite-миграция/зашифрованные config и channel/AAD, два подтверждённых baseline; B before A; доставка одного operation ID после close/open DB и повторной миграции; неверный bearer/digest; повтор ACK; конкурентный CAS start; восходящие ревизии и snapshot compensation A before B; B failure, failed compensation;8 доставок без ACK/attention/сохранение leases/явное восстановление; late ACK отвергнут; capability0 и withdrawal после start; roster CAS; блокировки direct deploy/user mutation/endpoint/delete route/legacy restore; отсутствие публикации/probe pass после двух ACK.

App integration создаёт настоящие typed REALITY configs, сохраняет channel, запускает журнал, получает команды через существующий node poll и проверяет их `noderuntime.ValidateRequest` в порядке B/A. ACK в этой fixture синтетический: он не выдаётся за выполненный Xray/systemd apply. Helper capability fixture подтверждает read-only/no-systemctl/no-file и отказ query с payload. Предыдущий настоящий Xray config/loopback тест относится к [неизменённому renderer](stage5-2026-11-06-cascade-channel.md), в этом блоке не повторялся.

## Не проверено

Процессный kill/reboot/root helper/systemd/SSH crash/rollback, настоящий REALITY A→B/B→A, два VPS, внешние IP/DNS/TCP/UDP, distributed revoke/accounting/limits и профиль512МиБ/1vCPU **NOT RUN**. Close/open DB — тест persistence, не доказательство crash consistency всей ОС. Нет admin UI/endpoint activation, LogicalEndpoint/user binding, channel rotation и независимой egress/publication. Stage4/5 не ACCEPTED.
