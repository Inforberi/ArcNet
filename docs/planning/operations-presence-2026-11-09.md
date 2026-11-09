# Операции, UX пользователей и presence — 9 ноября 2026

Активный запрос: полный план пользователя из чата (CRUD/key rotation, единая карточка/таблица, presence local+nodes, адаптивность/браузер/реальные локальные движки). Продакшен не менять. Диагностика серверного отказа отсутствует по выбору владельца: точную причину screenshot500 не объявлять известной. Новые протоколы/биллинг/IP enforcement не входят в объём. Применяется go-api-nethttp.

## Подтверждено исходниками

- connectionDelete не принимал additional connection IDs, хотя service/store умеют их удалять с revision guard.
- UserDetails скрывает удаление в Activity/Advanced; две конкурирующие навигации; sticky actions используют отдельный фон.
- API клиент склеивает server error с общим текстом unknown; часть mutations ожидают refresh в success-ветке без раздельного состояния.
- Add/Toggle/Delete пользователя, AddProtocol и Setup игнорировали отдельные ошибки компенсации. Это риск ложного сообщения о восстановлении, не доказательство причины screenshot.
- TrafficTracker использует counters, online:null. Xray StatsService приватный Unix socket; Hysteria Stats API защищён loopback/token; WG transfer helper. Presence отсутствует.

## Ход реализации

Блок1 (операции и базовый UX) реализован: дополнительный DELETE/CAS, MutationError и typed error outcome, bounded recovery с проверкой SQLite/runtime, отдельный refresh failure в Users/VPN/UserEditor. Targeted Go app/HTTP CRUD/rotation/rollback PASS; полные app и HTTP тесты PASS (первый app прогон выявил потерю внутреннего текста причины в MutationError, восстановлена Cause.Error, HTTP по-прежнему sanitizes). Frontend38 tests/TS/Vite PASS. Browser5 scenarios/10 widths320…1920 PASS,0 crashes: create/flat navigation/draft/pending save, actual PATCH с последующим injected GET failure, delete/reload, visible disabled connection delete/explanation/Escape focus,20 list views и10 editor layouts. Runtime fixture simulated, не VPN traffic/systemd acceptance. Стенд остановлен после проверки. Логи /private/tmp/arcnet-operations-*, screenshots/results /private/tmp/arcnet-operations-review.

Готов базовый UX: одна карточка Основное/Доступ/Лимиты и срок/Активность с одним draft/footer, native top-layer ActionMenu без новых зависимостей, row/card deletion и отключённый DELETE с объяснением,6-column table без sticky actions заливки, ротация серверных ключей вне вложенного раскрытия. Следующие обязательные блоки: local PresenceTracker/adapters; отдельный capability+presence report нод с revision/digest/epoch и совместимостью старых агентов; additive presence API/summary; browser E2E10 widths и настоящие local Xray/Hysteria клиенты; замер512МБ/1vCPU; финальная документация/коммиты.

Presence правила: online только свежий session source; offline только полный подтверждённый набор; counters/handshake = recent; устаревшие данные и отсутствие поддержки не offline. Node attribution только фактическая; connection_id не придумывать при общей Xray email. Poll общий5с, fresh20с, HTTP не запускает движки, IP/logging не добавлять. Финансовый учёт не менять.

При runtime checks разделять fixtures, браузер, реальные локальные движки и VM/VPS. Existing TestRealXrayStatistics на loopback не доказывает online: upstream OnlineMap исключает loopback, нужен отдельный изолированный non-loopback клиент. 3x-ui идеи только, код не копировать (GPL).
