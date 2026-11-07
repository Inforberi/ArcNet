# ArcNet: редактирование и навигация — 2026-11-07

## Исходные проверки

- Node22.16.0 `npm --prefix frontend run build` — TypeScript/Vite PASS после изменений редактора и общего меню; новых зависимостей нет.
- Linux image `arcnet-remote-tests:20261106`: `go test ./internal/app ./internal/store ./internal/httpapi` — PASS.
- После добавления regression cases и RuntimeCheck: `go test -race ./internal/app ./internal/httpapi -run 'TestConnection|TestAuxiliaryConnection' -count=1` — PASS. Проверены сохранение ключей/пользовательских tokens/credentials для TCP/XHTTP/Hysteria2/WireGuard, stale revision, конфликты портов/TLS, откат сохранённых параметров и Xray-файла при отказе validation, auth/CSRF, отключение/удаление и последняя local identity, paused runtime без изменения данных.
- Первые новые тесты выявили ошибки fixture (дважды созданный WireGuard и неверная CSRF-константа); fixtures исправлены, security assertions не ослаблялись.
- Настоящий React/API/SQLite browser fixture с simulated engines: регистрация/вход, новая таблица, редактирование SNI с diff и сохранением, отключение REALITY при сохранённой Hysteria2, пользователь/подписка без выбора приложения. Mobile390×844: document scrollWidth=390, dialog clientWidth=scrollWidth=390; доступны все действия. Desktop1280×720: проверено центрирование. Стенд штатно завершён через /fixture-done, `TestOnboardingBrowserFixture` PASS (165.22s). Это управляемое время browser-сценария, не нагрузочный тест или VPN handshake.

После последнего запроса пользователя отдельно проверена обновлённая общая SVG-навигация: порядок workspace/system links, настройки снизу, mobile390 без горизонтального переполнения, переход в настройки и автоматическое закрытие меню. Итоговые proof images: `/private/tmp/arcnet-protocols-preview.png`, `/private/tmp/arcnet-editor-mobile.png`, `/private/tmp/arcnet-navigation-mobile.png` (синтетические данные). Повторный стенд также остановлен штатно; `TestOnboardingBrowserFixture` PASS (76.25s).

Дополнительная целевая UI-проверка после новых требований: native mobile drawer, переход/закрытие, группы аккаунт/сервер/обновления, создание VLESS/XHTTP и переключение формы на WireGuard, lazy SNI-picker без запуска сетевого probe/применения. Mobile390 без горизонтального переполнения. Стенд завершён штатно: `TestOnboardingBrowserFixture` PASS (201.89s). После добавления названия подписки выполнены `go test ./internal/httpapi -run 'TestSubscriptionTitle|TestDevice|TestUserDirect|TestIndependentProtocol' -count=1` PASS и TS/Vite build PASS. Проверены ASCII-safe base64 header, кириллица/25-rune bound/control characters, delivery без изменения URI body. Реальное название в клиенте не проверено.

## Границы

Тесты используют fake engine validation/restart, не настоящий systemd, kernel WireGuard или внешний VPN-клиент. Новое редактирование/rollback на отдельной VM/VPS ещё не принято. На сервере пользователя Hysteria2 работает по его сообщению; этот исходный блок туда ещё не применён. Причина VLESS в Happ через мобильную сеть не установлена и диагностика отложена. Каскады, выбор протоколов конкретной подписки, график, полный updater и автоматические UFW-разрешения этим блоком не реализованы.

## Выпуск A6

Опубликован `v0.7.0-stage5-a6`, final core/UI source `7d7c9b3d6d80d9a7e0cc43c6174889090893429c`, source manifest SHA256 `5454ebc11a01564b41b35005d8ae6587384f73b8efdddf1a78a2697a97de16d7`. Предыдущий кандидат 7cfba38 не публиковался. Полный Linux/amd64 release pipeline PASS: pinned npm ci/TS/Vite, Go tests/vet, embedded frontend/version/lifecycle, packaging. Подпись installer проверена штатным offline verifier; update signature и точная версия/size/hash core независимо проверены Node crypto. SHA256 update-a6.zip: `753032258d156d14eb27e8630649e37e0fc576039159ebb5dc9b6619f5dfc937`. BUILDINFO dirty=true отражает исключённый untracked .serena; точный source snapshot приложен к релизу. Установка/rollback этого пакета на VPS ещё NOT RUN.

Публично скачанный update-a6.zip совпал с закреплённым SHA256. После bump pin выполнен offline `bash deploy/bootstrap-install.sh --verify-only --from-dir dist/installer-0.7.0-stage5-a6`: signature/pinned version/archive/payload/safe extraction PASS, install/runtime не запускались.
