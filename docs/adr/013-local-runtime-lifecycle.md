# ADR-013: единый локальный lifecycle и root coordination

2026-11-06, A1. [CLI-контракт](../ops/local-service-control.md), [фактические проверки](../testing/local-results/stage5-2026-11-06-local-lifecycle.md). Source verified; реальный systemd/VM/VPS NOT RUN.

## Решение

Wrapper и меню делегируют root `core lifecycle`, без дублирования systemctl-цепочек. `internal/lifecycle` владеет порядком/состояниями; runner и узкие callbacks дают тестировать отказы без изменения ОС. Компоненты и systemctl path фиксированы; адреса, units и shell-команды из HTTP не принимаются.

Перед root-командой wrapper проверяет capability через help, который обрабатывается до открытия БД в поддержанных прежних core. Контекстные subprocesses имеют отдельную process group; timeout/SIGINT/SIGTERM уничтожают группу, чтобы runuser не оставил дочерний writer. Crash/SIGKILL recovery остаётся внешним gate.

Root не читает arcvpn-writable SQLite: отдельные runtime-plan/prepare/apply запускаются через фиксированный runuser как arcvpn, проверяют identity/существующую БД до открытия, не выполняют миграции. RuntimePlan — ровно четыре required bool, без секретов. Apply переиспользует существующий reconcile, а не запускает потенциально устаревшую конфигурацию в обход desired policy.

Общий root flock `/run/arcvpn-lifecycle/lock` отделён от unit RuntimeDirectory: unlink во время удержания lock создал бы новый inode и нарушил сериализацию с updater. Прежний update lock сохраняется. Directory root0755 нужен для чтения marker пользователем сервиса; lock0600, unsafe owner/write/symlink/FIFO запрещены. Работа конкурентного CLI/update отклоняется, а не ставится в бесконечную очередь.

Root `paused` ставится до остановки панели, блокирует worker apply/helper restart/условия units/cert hook. Final counters снимаются до остановки движков; неподтверждённый хвост не становится нулём. Start восстанавливает socket и применяет текущую политику до открытия панели. Failure cleanup ограничен по времени; его ошибка отражается как active/unknown, а не ложный rollback PASS.

Маркер boot-scoped: остановка не меняет enable/disable и не выключает renewal/backup. После reboot действует существующий автозапуск. Persistent stop/autostart UI, новый updater migration framework, bootstrap и remote node lifecycle вне A1.

## Ограничения и альтернативы

Простая shell-цепочка отвергнута: она не координируется с worker, не проверяет фактические состояния и может поднять отключённый профиль. Новый постоянно работающий supervisor/очередь не нужны: используем существующие systemd/worker и одноразовый CLI.

Выбранные component restarts на время сериализации останавливают панель и reconcile; они не являются гарантией нулевого перерыва управления. Accounting incomplete/review gates сохраняются, бесшовный конечный пакет после смены epoch не заявляется. Полная внешняя проверка systemd/socket activation/ACME/update recovery/WG/NAT обязательна; новый core-only update не устанавливает нужные wrapper/units/tmpfiles/hook автоматически.
