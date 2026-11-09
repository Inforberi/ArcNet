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

## Блок2 — presence реализован, checkpoint перед окончательной приёмкой

Добавлены usage.PresenceTracker (bounds/TTL/identity/coverage), Xray native batch API через private socket/config/credential guards, Hysteria /online с прежними loopback/token/PID guards, optional latest-handshakes через существующий WG helper. Handshake не влияет на финансовые поля Counter и не означает online. App.RunUsage собирает presence в том же цикле независимо от accounting readiness; pause5с/fresh20с. Source failures/unsupported versions не offline, sources сортируются; сводка имеет expires_at, frontend проверяет TTL каждого источника.

Remote: отдельный presence_v1/capability/report и POST /api/nodes/presence. Heartbeat presence_protocol0…1, derived node_presence_capabilities table (не telemetry history/не финансовый формат), old agents0 сохраняют прежнюю конфигурацию. statsUserOnline добавляется в node config только после capability1. Новый helper принимает старую и расширенную stats-конфигурацию; новый agent при HTTP400 heartbeat повторяет idempotent heartbeat без нового поля для старого controller. Presence report bound к bearer/node, applied revision/digest/operation, process epoch/start/time и exact credential roster; malformed/missing online/foreign identities rejected. Отказ presence не блокирует успешный финансовый отчёт. Не готовые/старые ноды остаются unknown.

Unknown recovery теперь ставит runtimeUncertain и сбрасывает usageApplied: нельзя выдавать полный offline по непроверенному runtime или удалять подключение до подтверждения. Успешный общий apply/reconcile снимает barrier. Первый полный тест обнаружил latch после успешного ручного disable, исправлено в applyAll; stage2 lifecycle/restore снова PASS.

### Наблюдавшиеся результаты

- Полные Go tests usage/xray/engines/store/noderuntime/nodeagent/app/httpapi PASS; после последних barrier-правок app+HTTP повторены PASS. Логи /private/tmp/arcnet-presence-final-go.log (первый Stage2 FAIL), /private/tmp/arcnet-presence-final-app.log (исправленный PASS). До них /private/tmp/arcnet-presence-proof-go.log —8 packages PASS.
- Targeted race usage/HTTP: TestPresence/TestNodePresence/TestUserTrafficAPI PASS, /private/tmp/arcnet-presence-race.log.
- Frontend40 tests и TS/Vite PASS: /private/tmp/arcnet-presence-frontend-{tests,build}.log.
- Real Xray26.10.31 (/private/tmp/arcvpn-xray-v26.10.31/xray), isolated non-loopback VLESS: первоначально offline, настоящий proxy transfer, online,21сек без application traffic остаётся online, закрытие offline, остановка источника error. Docker --memory512m --cpus1 PASS23.33сек; /private/tmp/arcnet-real-xray-presence.log. Plain VLESS, не REALITY/VPS acceptance. Реальный RPC возвращает user>>>email>>>online: decoder исправлен и regression добавлена; loopback не использовался как online-клиент. Fixture directory явно0700, guards не ослаблялись.
- Real Hysteria app/v2.13.0, official asset SHA256907ba8c9693edb104b20582681fb7dc15639d5b64a9cbb616a7b539190a86691, /private/tmp/arcnet-hysteria-runtime/hysteria. Настоящие trusted-CA TLS/QUIC и SOCKS bytes, /online, idle client online после закрытия proxy stream, stop client offline, stop source error: PASS1.07сек при512МБ/1CPU. PID/private config/listener guards exercised, systemd wrapper NOT RUN. /private/tmp/arcnet-real-hysteria-presence.log. Также existing real Hysteria accounting/device revoke test PASS в первом прогоне; не повторять неизменённый PASS. Fixture Ed25519 certificate был отклонён TLS-клиентом; приведён к ECDSA P-256 как существующий рабочий fixture, без TLS bypass.
- Два nodes HTTP fixture: auth/cross-node/revision/stale/ownership/expiry и отсутствие начисления трафика PASS. Настоящие удалённые VPS не запускались.

### Точный остаток для продолжения

1. Финальная browser-приёмка нового presence UI: online/recent/offline/unknown/stale, partial summary/expiry, источники/ноды, старый агент; повторить operations_presence_e2e после новой telemetry/UI (первый блок5 scenarios/10widths относится к af1dde0, не ко всем последующим presence изменениям). При изменении старых UI suites обновить labels Профиль/Пакет и срок/Подключения/Параметры к новой плоской навигации: tests/ui_ux_{audit,layout}_e2e.mjs пока старые.
2. Закрыть расширенную матрицу операций: все4 типа create/edit/delete/rotation через браузер, negative HTTP/network paths, unknown/refresh/reload; сейчас unit/HTTP покрытие и основной CRUD browser есть, но полная матрица из задания не принята.
3. Измерить стоимость batch collection/snapshot и память/CPU на512МБ/1CPU. Успех двух runtime tests при этом лимите НЕ является benchmark всей панели или minimum RAM/max users. Сейчас отдельных benchmark цифр нет.
4. Дополнительные concurrency/agent fallback tests и source review по необходимости; полного release build/vet после всего блока не выполняли. WG handshake runtime/root systemd, реальные remote nodes/REALITY/VPS NOT RUN.
5. После оставшейся приёмки обновить handoff/status; новый релиз не публиковался. Последний опубликованный A10 не содержит эти операции/presence.

Следующий запуск fixture: Docker arcvpn-local-test:20261105, env ARCVPN_ONLINE_UPDATE_BROWSER=1 ARCVPN_UI_REVIEW=1 ARCVPN_VERSION_MANAGER_BROWSER=1, go test -count=1 -v ./internal/httpapi -run '^TestOnlineUpdateBrowserFixture$' -timeout31m, port18394. Завершать POST /fixture-done. Native Chrome и Playwright /private/tmp/arcvpn-browser/node_modules доступны. Реальные процессы helper закрывает по cleanup; временные контейнеры --rm. Продакшен не менять.
