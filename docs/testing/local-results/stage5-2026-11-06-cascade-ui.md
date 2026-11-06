## Завершение браузерной проверки — A3, 2026-11-06

Один свежий запуск существующего fixture без Go test cache: `/cascade-ui.test -test.run=^TestCascadeUIBrowserFixture$ -test.v -test.timeout=2m` в одноразовом Linux-контейнере; сценарий `tests/cascade_ui_e2e.mjs` через существующий Playwright и Chrome — **PASS**. Fixture завершился сам после теста (23,78 с). Проверены preview и invalidation, явное применение, ACK B→A, сохранение после reload, rollback A→B, удаление журнала и отсутствие горизонтального overflow390/1280. Production UI/API не потребовали исправлений. ACK синтетические; это не VPN-приёмка. Прежний browser-pending статус ниже исторический.

# Каскады из UI — сохранённый этап 2026-11-06

По просьбе пользователя работа остановлена для сохранения при4% батареи.

Реализованы session/CSRF API preview/apply/list/start/rollback/delete, React диалог предпросмотра/подтверждения и журнал с polling только во время операций. Привязка — стабильный endpoint ID для TCP/REALITY входной ноды, независимо от её адреса. Каскад меняет дальнейший серверный путь: клиентские адрес/порт/UUID/REALITY параметры и subscription token не меняются. App сохраняет целиком подтверждённый listener обоих узлов и отклоняет рассогласование с ожидаемыми доступами вместо их пересоздания. Межнодовые ключи отдельные. Preview token привязан к ревизиям/digests/roster/профилям/порту; prepare/start запускаются отдельно от сохранения черновика. Поддерживаются пока только две managed TCP/REALITY ноды, fail-closed, без резервов/device mode/конечных пакетов. Действующий трафик может прерваться при restart Xray.

## Проверено

- `npm run build --prefix frontend` — TypeScript/Vite PASS,141 modules. Последняя UI-сборка соответствует сохранённому frontend.
- `go test -race ./internal/httpapi ./internal/app ./internal/store -run '^TestCascade'` — PASS; последний прогон HTTP1.057s, app1.072s, store cached. Покрыты auth/CSRF всех новых методов, preview без записи в БД, stale/changed plan/явное подтверждение, реальные сгенерированные helper-valid configs в порядке B/A, неизменность listener/client UUID/subscription, приватность DTO/no-store, rollback и очистка журнала. Сохранённые production Go-файлы после этого прогона не менялись.
- Первый браузерный запуск не нашёл bundled Playwright Chromium; добавлен существующий параметр ARCVPN_CHROMIUM для установленного Chrome.
- Первый реальный UI запуск дошёл до preview, затем упал на exact locator числового поля (описание включено в accessible label); исправлен тестовый locator на spinbutton с regex.
- Повтор НЕ подтвердил UI: Go вернул **cached** результат opt-in fixture и не запустил сервер, browser получил CONNECTION_REFUSED. Это НЕ PASS браузера. Не повторять без `-count=1`.

## Продолжить отсюда

1. Один свежий локальный fixture: `ARCVPN_CASCADE_UI_FIXTURE=/output go test -count=1 -v ./internal/httpapi -run '^TestCascadeUIBrowserFixture$'` внутри существующего `arcvpn-local-test:20261105`. Монтировать repo read-only как `/workspace`, native exec tmpfs/cache как раньше, writable private temp в `/output`; публиковать38567 только на127.0.0.1. Fixture сам завершается после POST `/__fixture_done` или90с. Убедиться, что сервер запущен; не использовать старый fixture.json.
2. `NODE_PATH=/private/tmp/arcvpn-browser/node_modules ARCVPN_CHROMIUM='/Applications/Google Chrome.app/Contents/MacOS/Google Chrome' ARCVPN_CASCADE_FIXTURE_FILE=<private-temp>/fixture.json ARCVPN_CASCADE_SCREENSHOTS=<private-temp>/screenshots node tests/cascade_ui_e2e.mjs`.
3. Проверить preview390/1280, применение, B/A ACK, reload, rollback/delete. ACK в fixture синтетический, это не VPN. При overflow длинного статуса на мобильном вероятно понадобится scoped flex-wrap заголовка новой карточки; не менять layout без наблюдения. Исправить конкретные сбои, повторить только затронутый сценарий.
4. Обновить этот статус по наблюдавшемуся результату. Общие сборки/race/frontend матрицу без новых изменений не повторять.

Реальные systemd/две VPS/REALITY/egress/DNS/UDP NOT RUN. Статус после ACK — awaiting_probe, не подтверждённый VPN. Публикации нового маршрута/подписок и независимого cascade egress-runner ещё нет; текущие клиентские профили сохраняются. A2 prerelease не пересобирался. Source/UI этап сохранён, браузерная приёмка не завершена.
