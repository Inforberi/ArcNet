# A1 — локальное управление службами, 2026-11-06

Контракт: [local service control](../../ops/local-service-control.md). **SOURCE_TESTS_PASS / SYSTEMD_VPS_NOT_RUN**. Это не production acceptance и не готовый установочный Release.

## Среда и фактические проверки

Одноразовый Linux arm64 Docker `arcvpn-local-test:20261105`, Go1.26.6 и прежние SQLite/qrencode. Source mount read-only, GOCACHE отдельный. В fixture системные команды заменены узким in-memory runner/стабами; настоящие services/firewall/SSH не менялись.

```sh
bash -n deploy/arcvpn-menu.sh install.sh
go test ./internal/lifecycle ./internal/store ./internal/app ./internal/updater ./cmd/arcvpn
go vet ./internal/lifecycle ./internal/store ./internal/app ./internal/updater ./cmd/arcvpn
go test -race ./internal/lifecycle ./internal/updater ./cmd/arcvpn ./internal/app ./internal/store -run 'Test(FullLifecycle|StopFailure|PartialStop|StartFailure|CleanupFailure|LifecycleLock|ComponentRestart|UnavailablePolicy|GuardRejects|Runtime|ManagementWrapper|UpdaterRejects|InstallerRollback|InstallerUnits|CertificateHook)'
```

Все команды — exit0. Race: lifecycle1.020s, updater3.132s, cmd1.068s, app1.038s, store1.019s. После небольшого уточнения directory mode для root umask077/FIFO: `go test -race ./internal/lifecycle` PASS1.067s и `go vet ./internal/lifecycle` PASS.

Дополнительно `go test -race ./internal/app -run '^TestFinalUsageFailurePreservesKnownTotalsAndReviewGate$'` — PASS1.052s: отсутствующая финальная статистика не меняет известные totals/период/review gate/UUID/token. Production inputs после финального diagnostic build не менялись.

Финальные уточнения wrapper/core capability handshake и отмены process group проверены отдельно: `go test -race ./cmd/arcvpn -run 'Test(LifecycleExec|ManagementWrapper|LifecycleRejects|RuntimeCommands|RuntimePlanParser|InstallerUnits|CertificateHook)'` — PASS1.505s, `go vet ./cmd/arcvpn` и повторный diagnostic `go build` — exit0. Старый core получает только известный безопасный help, не lifecycle/root DB команду. При timeout дочерний shell с отложенной записью уничтожается вместе с группой, orphan-write не появляется. Обычные SIGINT/SIGTERM подключены к тому же контексту; SIGKILL/cgroup/crash recovery на VM не проверялись.

`bash -n install.sh deploy/arcvpn-menu.sh scripts/package-release.sh tests/release_packaging.sh` и `bash tests/release_packaging.sh` в одноразовом Linux — PASS: CLI-руководство включено в archive, tamper отвергнут, private/stale artifacts исключены. Это synthetic packaging fixture, не выпуск настоящего релиза.

`go build -o /output/arcvpn-core ./cmd/arcvpn` — PASS. Это diagnostic core без embedded release frontend/ldflags: `version` показал существующий default `0.6.5-stage4`, не новый release0.7. Smoke `arcvpn-core lifecycle restart sshd` отвергнут до системных вызовов, exit2. Пакет установки/полный Linux x86_64 release этим блоком не собирался.

## Покрытые риски

- Stop-order: panel → final counters → socket/helper → engines. Отказ остановки панели запрещает дальнейшие engine actions; unknown D-Bus не считается inactive.
- Start применяет desired policy перед открытием панели, disabled profiles не стартуют. Повтор start/stop идемпотентен; selected restart не останавливает другой движок.
- Ошибка apply закрывает частичный runtime и сохраняет paused. Ошибка cleanup явно оставляет active/error в отчёте; нет ложного stopped/PASS.
- Общий flock отклоняет concurrent CLI/updater до mutation. Updater не начинает чтение staged payload при paused/busy; исходный rollback updater покрыт прежним regression.
- Directory/marker/lock проверяют owner/mode/type, symlink и FIFO lock запрещены, root umask077 не лишает сервисного пользователя возможности проверять marker.
- Safe runtime plan читает только флаги; parser отклоняет unknown/incomplete fields, trailing data и вместо bool произвольные строки. Root не открывает центральную SQLite: runtime-plan/prepare/apply требуют service identity до filesystem creation.
- Paused apply отклоняется до изменения файлов; неудачный toggle восстанавливает пользователя/UUID/token. Final stats failure не должен обнулять totals/review gate.
- Wrapper и меню направляют общие/legacy команды в один coordinator, unknown unit/extra args отвергаются. Исходный wrapper dispatch fixture обновлён под root core: первая проверка старого теста ошибочно пыталась выполнить отсутствующий `/opt/.../core`; исправлен test-only stub, production path не перенастраивается из env.
- Извлечённый cert deploy hook в Bash fixture продолжает установку двух обновлённых файлов, но не вызывает try-restart при paused. Unit templates и tmpfiles/update write paths проверены на единый marker path; `bash -n` PASS. Это не реальное исполнение systemd.

## Что не выполнялось

Настоящие systemd jobs/socket activation/условия unit, signals/timeout/kill, WireGuard down/NAT, ACME renewal, managed old install migration, reboot, ограничение512МиБ/1vCPU и реальный VPN/egress/byte overshoot **NOT RUN**. Нет изменения frontend: npm/build/browser не запускались. Нет новых зависимостей или постоянно работающего collector.

Пользователь самостоятельно проверит runtime на отдельном тестовом VPS по [плану A1–A4](../../planning/pre-vps-install-and-cli-2026-11-06.md). При потере счётчиков/смене эпохи existing incomplete/review gates сохраняются; бесшовное исполнение конечного пакета после restart не подтверждено. Следующий подготовительный блок — A2, воспроизводимый installer asset и инструкция миграции. Bootstrap одной командой и каскады этим блоком не реализованы.
