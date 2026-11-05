# ArcVPN 0.5.1 — Code Handoff этапа 3

**Дата:** 2026-11-04. Полные исходники. **Не production, не проверено на реальном VPS.**

## Добавлено относительно 0.5.0

1. `internal/firewall/ufw.go` + `internal/httpapi/firewall.go` + `internal/ops/firewall.go`: включённые VPN-порты отображаются в UI, а root helper принимает лишь строго типизированные `allow/remove` с меткой `ArcVPN managed VPN`. Не применяется `ufw enable/reset`, не редактируется SSH/общий UFW, при неактивном UFW операции прекращаются с ошибкой.
2. `internal/updater/*`, `internal/httpapi/update.go`, `internal/ops/update.go`: пакет с Ed25519-подписанным manifest; при загрузке ограничен размер; подтверждение в UI; замена бинарников с `.previous` и восстановлением при сбое старта. Поддерживаются core, Xray, Hysteria2 и совместимый набор AWG3.1 (`awg`, `awg-quick`, `amneziawg-go`). Обновление на VPS требует **отдельно доверенного публичного ключа владельца релиза**, не создаётся автоматически на сервере. Приватный ключ хранится offline.
3. `internal/backup/*`, `internal/store/portable_nodes.go`, `internal/app/backup.go`: зашифрованный перенос backup v3; пользователи и секреты VPN, дополнительные протоколы, подписки, идентификаторы нод/хеши токенов и настройка бюджетов логов. Смена IP поддерживается через remap при import; клиенты должны переимпортировать подписку при смене URL, агенты должны знать новый адрес контроллера. Не переносим admin/hash/сессии/TOTP и старые IP-сертификаты.
4. `install.sh`: unit `arcvpn-update.service` с `RuntimeDirectory=arcvpn-update` и отдельным writable lock; namespace journald, TLS, backup timer, UFW helper; в runtime не нужны Node.js/npm.
5. `frontend/src/pages/Security.tsx`: UI изменения UFW, подготовленного обновления, загрузка и подтверждение подписанного release, отмена staging, TOTP и логи; `Backup.tsx` — перенос v3.
6. `internal/updater/apply.go`: безопасное чтение stage ZIP (один файловый дескриптор, отказ для симлинков, special files и oversized sparse archives) до десериализации root-процессом.

## Фактические локальные проверки

См. [`testing/local-results/stage3-2026-11-04.md`](testing/local-results/stage3-2026-11-04.md). Пройдены обычные Go tests, vet, Bash syntax, выделенные race tests для хранилища, backup, firewall и updater, HTTP negative tests, TS/TSX syntax parse. **Не проверены** TypeScript typecheck, Vite build, браузерные E2E, реальный systemd, UFW, Xray/QUIC/WG traffic, ACME сертификат, обновление на VPS.

## Внешние блокирующие условия для Codex

- Закрепить `frontend/package-lock.json` на машине с npm, выполнить `npm ci`, `npm run typecheck`, `npm run build`, исправить TypeScript/React ошибки, прогнать E2E на 320/390/768/1024/1440.
- Go 1.25+ и сеть для pinned AWG3.1, собрать release через `scripts/build-release.sh` и выполнить настоящую установку на **отдельном** Debian/Ubuntu VPS.
- Включить UFW с заранее гарантированным SSH доступом, испытать операции UI и аварийный откат; проверить ограничения `ProtectSystem`/root helper, таймер сертификатов, журналы, backup и signed upgrade/revert.
- Проверить каждый протокол VPN из реальной сети клиента, экспорт/импорт и новую подписку на другом IP.
- Сохранить конкретные результаты в `docs/testing/external-results/`, обновить статус и только после всех PASS принимать этап.

**Моя часть (локальный Go + исходники UI): выполнена в пределах доступной среды. Проверка Codex: ожидается. Этап 3 ACCEPTED: нет.**

## Ретроспективная поправка к полноте scope (аудит 2026-11-04)

Проверка полного ZIP выявила, что исходный roadmap этапа 3 предусматривает управление SSH из UI, а `0.5.1` реализует только UFW-политику без SSH mutation API. Поэтому надпись «моя часть выполнена» выше относится только к перечисленным в handoff компонентам и **не доказывает выполнения всего исходного этапа 3**. См. [аудит](audits/2026-11-04-pre-stage4.md), [текущую задачу](ai/current-task.md) и `S4-00a` в [чеклисте](planning/stage4-checklist.md).
