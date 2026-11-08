# Таблица / batch / расписания — 8 ноября 2026

Только локально. Релиз, VPS, SSH и UFW не менялись.

## Проверки

- `go test ./internal/usage ./internal/store ./internal/app ./internal/backup ./internal/httpapi -count=1` в существующем Linux image, network none: PASS (0.05/2.34/3.62/1.30/12.06с).
- Новые проверки: атомарность batch при missing/stale/duplicate/ошибке keygen, уникальные credentials, auth/CSRF и CAS API, numeric sorting/unknown-last, CSV escaping/no UUID, календарные DST23/25ч и февраль, stale reset/catch-up/idempotency/clock rollback, encrypted backup15 и запрет schedule в14.
- `npm test`:16/16 PASS.
- TypeScript/Vite build: PASS.
- `git diff --check`: PASS.

Browser adaptive/modal acceptance NOT RUN: управление браузером запрещено Work settings. Локальный fixture18394 — только UI/API с mock engines, без настоящих VPN и коллектора статистики; в нём finite quota может быть недоступна из-за отсутствия fresh counters, runtime=unverified ожидаем. Календарная логика и сохранение расписаний покрыты тестами, реальный длительный RunUsage/systemd/reboot приёмочный сценарий не выполнялся.

Local-only guards сохраняются; remote quota/schedule enforcement не заявлен. Новые exports имеют backup15; прежние сборки его не читают.

После финального переноса coverage-guard внутрь write-транзакции повторены только затронутые `store/app/httpapi`: PASS (1.82/3.39/11.66с). Финальная TS/Vite сборка и `git diff --check` PASS. HTTP-тест также сохраняет monthly/IANA schedule через editor JSON.
