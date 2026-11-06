# ADR-019: расход прямых TCP-нод в общем журнале

2026-11-06. Реализован source-блок удалённого учёта. Конечные удалённые пакеты и сроки по-прежнему запрещены до автономного исполнения budget/revoke на ноде.

## Контракт

- Root-helper сообщает отдельную `usage_protocol=1`, а heartbeat обновляет её срок доверия. Только отдельный `apply_xray_usage_v1` допускает строго фиксированные StatsService/policy и приватный Unix socket `/run/arcvpn-node-xray/xray-stats.sock`; старые apply-контракты API не получают. HandlerService, TCP API и произвольные paths/RPC запрещены.
- У новой Xray-unit есть собственный RuntimeDirectory0700 и UMask0077. Сборщик проверяет владельца директории/socket (`arcvpn-xray`), receipt/config digest, socket epoch и неизменность config/receipt/epoch вокруг bounded запроса. UUID и другие private config поля в отчёт не входят. Учёт пользователя — не сетевые метрики VPS.
- `statistics_v1` возвращает только имена учётных записей и абсолютные upload/download плюс operation/revision/digest/epoch/time. Device email содержит owner ID; UUID доступа и клиентские ссылки не меняются. Агент привязывает report к своему NodeID/индивидуальному bearer. Отдельный cancellable collector каждые5с не зависит от интервала UI и долгого apply; ошибка использует ограниченный backoff.
- `POST /api/nodes/usage`: body<=128КиБ, counters<=512, authentication и применённая ревизия проверяются в одной транзакции. Ревизия queued/running, другая нода, чужие/дублированные email, регрессия, retired epoch и stale/future report отклоняются. Допускается только прямой managed TCP профиль без cascade lease. Сервер сам восстанавливает roster из защищённой конфигурации и ownership из БД; report не назначает владельца расхода.
- App использует прежний mutation lock, SQLite владеет одной транзакцией для checkpoints/пакетов/списаний. Локальные и удалённые источники разделены `Counter.Source`/`StorageStream`. Повторы не списывают вновь, общий расход суммируется по источникам, reset сохраняет checkpoints. Новая epoch без подтверждённого final handoff помечает incomplete/needs_review: точность хвоста при restart не обещается.
- Portable backup v12 сохраняет Source; v1–v11 читаются, новый shape под старой версией отклоняется. Restore сохраняет high-water, не превращает старый node ACK в действующий. После удаления identity ноды её checkpoints очищаются, суммы пользователей остаются.
- Cascade lease отключает автоматическую direct-реконфигурацию. Откат каскада сохраняет исходный typed operation и возвращает accounting config; это проверено API-тестом. Учёт самого каскада и cascade devices не завершён.

## Граница доверия и остаток

Root-helper/агент/Xray доверены: bearer/revision не являются аппаратной аттестацией расхода. Report не доказывает физический VPS/VPN handshake. При любом неучтённом источнике source_fresh становится false, известный расход сохраняется. Новые настройки не нужны.

Нельзя снимать `CheckPackageCoverage`/finite guards на основании одних успешных отчётов. Следующий необходимый блок: непересекающиеся budget allocations, автономное ограничение/expiry при потере панели, подтверждённый stop/revoke/reboot и согласование периодов/restore. Remote UDP, cascade accounting/devices, Geo/DNS, клиенты, domain/SSH/full updater остаются открытыми. Доказательства: [проверки](../testing/local-results/stage5-2026-11-06-remote-usage.md).
