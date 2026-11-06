# Активный остаток runtime — 2026-11-06

Прямое поручение пользователя: реализовать весь остаток, проверки выполнять после готовых реализационных блоков. Не заменять фичи заглушками/планами. Не терять неизменность клиентских профилей при изменениях каскада. Краткие targeted проверки по риску; реальная инфраструктурная приёмка отдельно.

Порядок и критерии:

1. Каскады: внешний runner, exit IP + DNS через туннель, one-time report grant/revision binding, отдельная публикация, отзыв при stale/изменении/rollback. Не выдавать runner self-report за аттестацию или DoH за leak-test устройства.
2. Remote устройства/учёт/лимиты: scoped per-device credentials, подтверждённая телеметрия с epoch/revision, monotonic/idempotent debits, исполнение revoke/finite limits с ACK и неопределённым результатом. Не снимать существующие запреты до полного сценария.
3. GeoIP/GeoSite и DNS: bounded authenticated import/verify, доступные источники/версии, понятный preview, validated runtime apply/rollback без скрытого device direct.
4. Параллельный failover: доверенная сеть клиента/проверенный формат экспорта; bounded parallel rounds, hysteresis/cooldown/cancellation и fail-closed. Панель не может видеть доступность из сети абонента.
5. Поздний домен/SSH-hardening: актуальная threat model, plan/diff, staged rollback и проверка альтернативного SSH до commit. Privileged deployment приёмка только в изолированной VM.
6. Полная обвязка updater: подпись полного manifest, core/helpers/wrappers/units/hooks согласованы, backup/version/DB compatibility, restart recovery/компенсация. Не выдавать core-only updater за эту функцию.

Исходная точка: a5443d5, UI apply/rollback сохранён, frontend/API targeted PASS, browser acceptance остаётся открытой. A2 Release не содержит новых source-блоков. Тестовых VPS нет, user выполняет внешнюю приёмку. Этап6/новые протоколы и AmneziaWG development не входят в этот объём.

Уточнение пользователя: клиентами должны быть Happ, Streisand и другие стандартные приложения, не собственный клиент. URI subscriptions/credentials сохранять стандартными. Нельзя выдавать стандартный список профилей за программу failover: поддержка параллельных проб/приоритетов/порогов зависит от конкретного клиента/версии; неизвестные возможности не включать молча.
