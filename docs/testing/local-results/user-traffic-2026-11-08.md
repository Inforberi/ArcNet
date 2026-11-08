# Телеметрия пользователя — 8 ноября 2026

Только локальные исходники. VPS/релиз не менялись.

- Linux image `arcnet-remote-tests:20261106`, `go test ./internal/usage ./internal/app ./internal/httpapi ./internal/store -count=1`: PASS (0.03/3.20/11.28/1.39с).
- Targeted `go test -race ./internal/usage -run TestUserTraffic -count=1`: PASS.
- Новые случаи: warmup/no false initial activity, сумма потоков, missing source не ноль, recovery baseline, stale, process epoch, clock rollback/duplicate timestamps, identity/deletion, bounded ring/concurrent snapshot, admin auth/no-store/no UUID-token, bounded200 IDs и невозможность запуска collector на GET.
- `npm test`:19/19 PASS. В том числе график с gaps/epochs, freshness с различием часов и старые DS/таблица tests.
- TypeScript/Vite build и diff whitespace: PASS.

## Короткий замер, не VPS benchmark

`go test ./internal/usage -run '^$' -bench BenchmarkUserTraffic -benchtime=10x -benchmem`, linux/arm64:

| Операция | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| Update1000 |595112|848581|1020|
| Snapshot50 |2379|4096|1|

Это только обработка/чтение RAM-снимков, без SQL/HTTP/движков, всего10 итераций. Не выводить отсюда максимум пользователей, RAM VPS или общую скорость системы.

Browser acceptance NOT RUN (Work settings запрещают browser control). Hidden/offline polling и page/modal UX реализованы, но не проверены реальным браузером в этом блоке. Стенд18394 mock-only, реальная статистика и туннели отсутствуют. Online/session/IP identification и VPN handshake не заявлены. Кольцо истории не переживает reboot; авторитетные пакеты/расход/backup15 не менялись.

Финальные проверки после ограничения длины ids и обработки часов: `go test ./internal/usage ./internal/httpapi -run TestUserTraffic -count=1` PASS (оба0.02с). Финальная TS/Vite сборка и `git diff --check` PASS. Предыдущий preview завершился по времени жизни; поднят новый mock-only стенд18394. Это не браузерная приёмка.
