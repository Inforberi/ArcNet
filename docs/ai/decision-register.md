# Реестр продуктовых и технических решений

| Решение | Статус | Где подробно |
|---|---|---|
| Go + SQLite + Xray | Зафиксировано | ADR-001 |
| React/TypeScript/Vite + React Router Data Mode | Зафиксировано | ADR-001 |
| Tailwind + выборочные shadcn/ui, component composition | Зафиксировано | design-system.md / composition.md |
| Standalone без отдельного агента | Зафиксировано | overview.md |
| Один VPS 1vCPU/1GB как целевая среда | Гипотеза, требует benchmark | acceptance.md |
| Подключение VPS через UI/SSH | Требование, не реализовано | nodes-and-installer.md |
| Отсутствие обязательного собственного домена | Зафиксировано | ADR-003 |
| HTTPS по IP | Требование, требуется POC renewal | server-security.md |
| Каскад любого направления или отсутствие каскада | Зафиксировано | routing-failover.md |
| Резервирование egress | Требование, не реализовано | routing-failover.md |
| Резервные подписки | Требование, зависит от клиента | subscriptions.md |
| Limit HWID | Отложено, ненадёжное identity | users-and-devices.md |
| Docker/Podman | Только optional | ADR-001 |
| React UI как в 3x-ui по характеру, не копия CSS | Зафиксировано | screens.md |

| Схема и таблица серверов (одни данные) | Требование, согласуется UX | design/topology-and-table.md |
| Полная автоматизация с advanced overrides и provenance | Требование, проектное решение | features/automation-and-advanced.md |
| SSH host-key verification / preflight / rollback обязателен | Security requirement | features/nodes-and-installer.md |
| Матрица поддерживаемых ОС/минимальных ресурсов | Требует стенда и измерений | ops/server-requirements.md |
| Безопасность без shell RPC у удалённых агентов | Требование | security/threat-model.md |

## Решения от 2026-11-04

| Решение | Статус | Документ |
|---|---|---|
| Импорт 3x-ui исключён; нужен безопасный перенос ArcVPN → ArcVPN | Зафиксировано | `docs/features/backup-and-migration.md` |
| Пишем код здесь, внешнюю проверку выполняет Codex | Зафиксировано | `docs/planning/execution-boundaries.md` |
| `LOCAL_PASS` != `ACCEPTED`; ни один этап не принимается только на mock тестах | Зафиксировано | `docs/planning/roadmap.md` |
| Серверные probe не определяют доступность клиента и не создают универсальный auto-switch | Зафиксировано | `docs/architecture/failover-client-reachability.md` |
| Протоколы приоритетные: VLESS/Reality TCP/XHTTP, Hysteria2, WireGuard, AmneziaWG; дополнительные через отдельный gate | Зафиксировано как roadmap, не реализовано | `docs/features/inbounds-and-protocols.md` |

## Решение от 2026-11-05 — заморозка перечня протоколов

| Решение | Статус | Документ |
|---|---|---|
| Использовать существующие пять VPN-вариантов: VLESS/Reality TCP, VLESS/Reality XHTTP, Hysteria2, WireGuard, AmneziaWG 3.1 | Зафиксировано; внешняя приёмка остаётся открытой | `docs/planning/protocol-scope-freeze-2026-11-05.md` |
| Этап 6: AnyTLS, Trojan, TUIC, Shadowsocks 2022, экспертный режим/overrides | **Отложено** до явного решения пользователя; не в текущем scope | `docs/planning/protocol-scope-freeze-2026-11-05.md` |
