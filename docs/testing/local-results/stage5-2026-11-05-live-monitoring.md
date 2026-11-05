# Проверки: live monitoring и connection forms

Дата: 2026-11-05. Только локальные source/UI/API checks. VPS/VPN/systemd/UFW/ACME **NOT RUN**. Ни stage4, ни stage5 не ACCEPTED.

## Среда

- Linux arm64, Docker `golang:1.26`, Go 1.26.3. Исходники read-only; SQLite/qrencode Debian libraries распакованы в одноразовое окружение, не установлены на хосте. Go temp/output отдельно.
- Frontend Node 22.16.0, реальные зависимости из `frontend/package-lock.json`, React Query 5.90.1, Vite 7.3.7.
- Playwright/Chromium 156 (v1248) на macOS arm64; настоящий Go API/SQLite/auth/CSRF через loopback proxy, без подмены API. Для configured-формы baseline Reality синтетически записан в остановленную disposable DB. Это **не** рабочие ключи/применённый Xray.

## Фактически выполнено

```sh
go test ./...
go vet ./...
go test -race ./internal/store ./internal/httpapi ./internal/nodeagent ./internal/metrics
npm ci --no-audit --no-fund
npm run build
npm test
bash -n tests/run-monitoring-browser.sh
git diff --check
```

Go test/vet — exit 0. Race: store 2.337s, httpapi 102.442s, nodeagent 1.252s, metrics 1.016s. После добавления тестов отмены in-flight отчёта и границы 5 секунд повтор `go test -race ./internal/store ./internal/nodeagent` — PASS (2.317s/1.264s); затем общий test/vet повторно exit 0. Frontend — 10 tests PASS; TypeScript + production Vite build PASS, 137 modules. После исправлений browser offline и доступных имён select повторный build PASS.

Проверены: session/CSRF на настройках, default5, persist15, запрет прочих значений; транзакционный rollback настройки при ошибке аудита; same-session SSE initial refresh/settings update/revoke/reconnect401; ограничение 32 stream и coalescing 100 notifications; trusted-origin defaults без Host poisoning и конфликт TCP443; optional telemetry200/204/404/429, Retry-After bounds, неверный cadence, отмена in-flight отчёта; приём через5сек и отказ через4сек. Существующие stale resources/unknown/VPN separation/retention/credential rotation tests также проходят.

## Браузер

```sh
node tests/monitoring_e2e.mjs
ARCVPN_E2E_CONFIGURED=1 node tests/monitoring_e2e.mjs
ARCVPN_E2E_CONFIGURED=1 ARCVPN_E2E_MONITORING=1 node tests/monitoring_e2e.mjs
node tests/dashboard_e2e.mjs
node tests/traffic_profiles_e2e.mjs
node tests/routing_e2e.mjs
```

Каждая команда запускалась с `ARCVPN_E2E_URL`, приватным `ARCVPN_E2E_PASSWORD_FILE`, `NODE_PATH`, `ARCVPN_CHROMIUM`; значения секретов не выводились. Все перечисленные итоговые прогоны — exit 0.

- Подписка активна, 5→10→5, persist после reload, две вкладки видят общую частоту, настоящий новый sampled_at без ручного Refresh.
- CDP offline → видимое состояние восстановления, online → живая подписка. Первоначальный smoke без ожидания offline был недостаточен; усиленная проверка выявила отсутствие offline/online handler. Исправлено, усиленный прогон PASS.
- Автозаполнение trusted IP/443; Advanced закрыт по умолчанию; ручной target не стирается. TCP/UDP default порт, сохранение изменённых TCP/UDP/SNI drafts при переключении XHTTP/WG/Hy2 и возврате. Hy2 не получает произвольный SNI.
- Пять размеров 320/390/768/1024/1440, light/dark, отсутствие горизонтального overflow/JS errors. Setup-dark390 и configured-light390 просмотрены визуально. Скриншоты в ignored `tests/screenshots/monitoring/`.
- Профили трафика E2E PASS: настоящий CRUD/import/export/off/CAS, draft retention, пять размеров/две темы, отказ SSRF-кандидата. Прежний тест пытался открыть вложенный SNI-каталог в свёрнутой форме; шаг обновлён, полный повтор PASS.
- Routing E2E PASS: pointer connections, reserve order, циклы/CAS, зависимости удаления нод, encrypted backup v5 preview, 8 страниц × 5 размеров × 2 темы, без JS errors. Это проверка черновиков, не каскада на сети.
- Старый dashboard E2E PASS: 23 синтетических ноды + local, legacy/offline/service/probe separation, история с настоящими минутными bucket, обе темы/пять размеров. SSE сохраняется при ожидании истории.

`tests/run-monitoring-browser.sh` добавлен для повторяемых Linux fixtures; в этой среде проверен `bash -n`, а сами browser-фазы запускались через Docker backend/macOS Chromium, не этим wrapper целиком.

## Встреченные ограничения среды

Первичный macOS Go run: отсутствует Linux `libqrencode.so.4`, sandbox запрещал test sockets. Первичный npm без network escalation завершился ENOTFOUND; установка с разрешённой сетью прошла. Docker root disk заполнен; пользовательские образы/контейнеры/volumes не очищались. Пакеты/cache размещены в RAM/одноразовом host temp. RAM mount оказался noexec, test binaries и runtime libraries перенесены в исполняемый temp bind. Это ограничения первых попыток, не замаскированные PASS. Итоговые Linux проверки выполнены после исправления окружения.

Автоматические проверки target могут обращаться к публичным кандидатам; успешный выбор в реальной сети и VPN-handshake этим UI-тестом не подтверждены. Нет теста реальной fleet новых агентов/применения периодов на VPS, quota accounting, устройств, каскадов или throughput.
