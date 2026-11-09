# UI/UX-аудит и frontend-рефакторинг — 9 ноября 2026

Исходная точка: `164acb2`. Объём — только React/CSS, существующие API и функции; палитра и VPN-конфигурации сохраняются. Статусы ниже основаны сначала на исходниках; браузерные результаты дополняются после запуска. Старые source-PASS не доказывают поведение текущего интерфейса.

## Матрица дефектов перед изменениями

| ID / приоритет | Экран, компонент и сценарий | Причина и последствие | Исправление / файлы | Проверка |
|---|---|---|---|---|
| 01 P0 | Пользователь: изменить имя, обновить query editor другой версией | key содержит preferences.version; размонтирование стирает draft | UserEditor: baseline на время редактирования, явный конфликт | Refetch с новой версией сохраняет поля |
| 02 P1 | Маршруты: правки → каскады → правила | условный mount уничтожает форму | Routing: сохранять открытые панели | Возврат сохраняет draft |
| 03 P1 | Создание пользователя: серверная ошибка | error находится за modal/inert | Users: локальная ошибка формы | 422 видна в диалоге; ввод сохранён |
| 04 P1 | Выдача доступа: QR/clipboard error | сообщение за modal; GET можно запускать параллельно без ownership | Users/Devices/DirectAccess: локальный статус, request ownership | Ошибка QR + копирование, быстрый выбор двух пользователей |
| 05 P1 | Любая mutation: потеря ответа | fetch TypeError не объясняет неизвестный результат, нет deadline | api: bounded timeout, outcomeUnknown, отсутствие auto retry; опасные редакторы блокируют повтор | Разрыв ответа не сообщает ложный успех/отказ |
| 06 P1 | Подключение: редактирование транспорта/защиты | длинный disclosure смешивает всё; Flow повторён | ConnectionEditor: секции с одним draft, одним apply, без второй строки Flow | Переключение секций и invalid hidden field |
| 07 P1 | Подключение: «Обновить параметры» | название обещает изменение вместо открытия SNI | Table/Editor/Details: «Подобрать SNI» | Действие открывает существующий picker |
| 08 P2 | Подключения: чтение таблицы | повтор TCP/REALITY, «Трафик профиля: —», «Без SNI» у WG | Table: убрать неполезные повторы, сохранить детали | Доступность всех технических полей в details/editor |
| 09 P1 | Все dialogs: двойное закрытие dirty | async confirmation без синхронного guard; confirm autofocus может попасть на опасное действие | Dialog/Confirmation: serial dismiss, initial cancel focus | Escape/outside, dirty cancel, focus return |
| 10 P1 | Все страницы: загрузка/недоступность | ошибки иногда нейтральный Notice без retry; Loading не объявляется | ui/ErrorState: alert/retry в контексте; Loading status | 403/500/empty различимы, navigation доступна |
| 11 P2 | Users/Devices: копирование | постоянный banner двигает layout; нет dedupe | компактный один transient status; ошибки остаются локальными | Повтор copy, dismiss, pause, mobile/reduced motion |
| 12 P1 | Backup: выбрать другой файл во время чтения | старый File.text может заменить новый; формы имеют общие ошибки | Backup: generation guard и отдельные ошибки export/import | Быстрый выбор файлов, ошибка пароля не затрагивает export |
| 13 P1 | Settings: MFA/account и server forms | вся доступная ширина; ошибки далеко от операции | Security/Account: ограничить ширину форм, контекст ошибок, field feedback | Секции/пароли/403/2FA, desktop/mobile |
| 14 P1 | Overview: telemetry request fail | устаревшее имя и исчезновение/empty вместо ошибки; refresh без контекста | Overview: независимые ErrorState, не выдавать cached значения за fresh | Telemetry/fleet/history отказывают отдельно |
| 15 P2 | Nodes/Events/Routes/Versions | разные error/retry и перегруженные secondary notices | использовать контекстные состояния, убрать дублирующий add server | Load/error/empty и сохранность технической диагностики |
| 16 P1 | Общий CSS/dialogs/формы | повторные owners style.css/components.css, nested scroll, ширины/отступы переопределяются | канонический владелец primitives, responsive workspace geometry | Все указанные размеры, dialog/footer/zoom/no overflow |
| 17 P2 | Фокус/control/loading | текст loading меняет ширину кнопок; select/textarea focus слабее Button | Button pending overlay, токены motion/spacing/focus | Keyboard, reduced motion, stable bounds |
| 18 P1 | Query errors/session expiry | глобальный retry повторяет 401/403/429, AuthGate размонтирует редактор | без retry для 4xx, reauth overlay сохраняет mounted workspace | 401 без потери draft и без запросного шторма |

## Правила композиции

- Частые действия доступны в строке. Просмотр и редактирование — самостоятельные входы, безопасный просмотр без подтверждений.
- Сложный редактор: один baseline/draft, локальная навигация по секциям, один атомарный submit, обзор изменений перед применением. Не скрывать ошибки в закрытой секции.
- Ошибка операции остаётся рядом с формой; неизвестный результат требует сверки существующего GET перед повтором. 409 не перезаписывает draft.
- Короткое подтверждение копирования не двигает страницу. Критическая ошибка не исчезает по таймеру. Опасные операции сохраняют подтверждения.
- Общие размеры/фокус/семантика, но разные композиции по контексту. Палитра tokens.css сохраняется.
- Проверка в браузере: реальные React/API fixtures с simulated VPN engines; network fault injection проверяет frontend, не серверное исполнение.

## Результаты

Исправления реализованы в настоящем frontend. Backend, JSON payloads/API endpoints, БД, генераторы конфигураций и палитра не менялись. Новых зависимостей нет; `npm ci` восстановил уже закреплённый Radix после pull.

### Что изменено

- 01–02: baseline редактора пользователя фиксируется на время сессии; обновление Query больше не пересоздаёт форму. Новая версия/409 блокирует перезапись и оставляет draft; есть сверка существующим GET. Панели маршрутизации сохраняют локальное состояние; тяжёлая схема загружается только после первого открытия.
- 03–04: создание пользователя и выдача доступа имеют ошибки внутри текущего диалога. GET выдачи имеет ownership; ответ после закрытия/смены запроса не открывает старую выдачу. QR failure и clipboard failure доступны рядом с кодом и ссылкой, не за inert-слоем.
- 05: GET deadline20s, mutation/upload120s включает чтение body; finally освобождает таймеры/listeners. Сетевая ошибка/нечитаемый mutation response/5xx не объявляет отсутствие применения. Основные редакторы, создание, rotation, bulk create/access и backup блокируют опасный повтор после unknown. Отмена браузерного ожидания не называется отменой работы сервера. Автоматических mutation retries нет.
- 06–08: подключение получает секции «Основное», «Транспорт»/«Клиентский файл», «Защита и SNI», «Ключи»/«Сведения». Один draft, один submit, сохранены все прежние поля и diff. Убраны повтор Flow, дублирующая info-кнопка (details остаётся на имени/строке), повтор TCP у endpoint, «Трафик профиля: —», «Без SNI» у WG. SNI action назван фактическим действием. Основной порт не растягивается на всю ширину.
- 09–10: закрытие dirty-dialog сериализовано; autofocus подтверждения — на отмене, focus возвращается к opener. Loading объявляется status. ErrorState различает загрузку и отсутствие данных, оставляет независимую навигацию; 403 не предлагает бесполезный retry, 429 учитывает Retry-After. Ошибки операции не исчезают по таймеру.
- 11: один компактный transient Feedback с явным dismiss, pause при focus/hover, без очереди одинаковых сообщений. В modal сообщение размещается внутри текущего focus/ARIA-контекста, root-копия не дублируется. Копирование прямой ссылки подтверждается локально; QR failure не создаёт второй alert. Sonner не добавлен: существующий небольшой набор подтверждений не требует ещё одного UI/runtime пакета.
- 12: export/import errors backup независимы; file-generation guard не позволяет позднему чтению старого файла заменить новый. Во время preview/restore входные данные заблокированы; unknown restore не повторяется. Подтверждение восстановления называет последствия и действие.
- 13–15: формы settings имеют ограниченную ширину, ошибки сохраняются по разделам, независимая навигация не блокируется чужой mutation. Совпадение пароля проверяется рядом с полем. Overview, Nodes, Events, routing profiles и releases получили контекстные ошибки загрузки. Убрано второе постоянное «Добавьте сервер»; diagnostics и ручная регистрация сохранены.
- 16–17: удалён прежний owner геометрии Dialog из style.css; базовая геометрия теперь в components.css. Editor navigation/title/footer закреплены в рабочей области с одним основным scroll owner. Заголовок не перемещается при смене секции. Pending Button сохраняет геометрию исходной подписи. Общие spacing/z-index токены, visible focus, reduced motion и safe-area используют прежнюю палитру. html резервирует scrollbar gutter; единственный новый `!important` компенсирует inline margin стороннего Radix scroll-lock, чтобы не резервировать его дважды.
- 18: повторный вход проходит поверх сохранённого workspace; drafts остаются только в памяти. После подтверждённого входа перечитываются существующие queries, CSRF/session/security модель сервера сохранена. 4xx не вызывают фонового retry-шторма.

Дополнительно визуальная проверка нашла наложение подписей мобильных user filters: вместо сжатых четырёх кнопок — сетка2×2. Sort получает читаемую ширину; нечётная последняя сводка занимает ряд, без пустой ячейки. Field сохраняет стабильное accessible name при появлении native validation; ошибка связана отдельно через describedby и aria-invalid.

### Действия до/после

Считаются нажатия от уже открытого списка; ввод значений и сетевое ожидание исключены.

| Сценарий | До | После |
|---|---|---|
| Открыть редактор подключения или пользователя | 1 | 1; прямое действие сохранено |
| Найти Flow или XHTTP transport в editor | открыть editor → раскрыть общий Advanced → прокрутить | открыть editor → «Транспорт»; 2 нажатия сохранены, длинная прокрутка убрана |
| Подбор SNI из строки | 1, но «Обновить параметры» | 1, «Подобрать SNI»; действие больше не обещает автоматическое обновление |
| Открыть дополнительный WG client field | editor → Advanced → прокрутка | editor → «Клиентский файл» |
| Получить подписку/URI и QR | «Подключить», затем при необходимости выбрать способ | столько же; pending/error видны сразу в открытом диалоге |
| Ошибка копирования / создание user422 | закрыть/выйти из modal, чтобы прочесть сообщение за ним | 0 дополнительных переходов |
| Вернуться в черновик маршрута после другой вкладки | повторный ввод потерянной формы | 1 переключение, введённые данные сохранены |

Просмотр и редактирование не объединены насильно; технические сведения, отдельные URI, QR, подписки и .conf сохранены. Нового wizard нет.

### Наблюдавшиеся проверки

- `npm run build --prefix frontend` — TypeScript project build + Vite PASS. Lint script в package.json отсутствует.
- `npm test --prefix frontend` — 36/36 PASS, включая новые transport unknown/deadline/cancel/Retry-After/retry-policy tests. Прежний strip-only Node выявил constructor parameter-property; класс переписан эквивалентными readonly fields без смены контракта.
- `tests/ui_ux_audit_e2e.mjs` — 11 сценариев PASS, 99 page/width сочетаний, 0 frontend crashes. Реальный Go HTTP/SQLite opt-in fixture, login и mutation; simulated VPN engines. 422/409/connectionreset/401/503/QR/clipboard и preview failure injected через Playwright.
- `tests/ui_ux_layout_e2e.mjs` — 5 дополнительных сценариев PASS: labels без clipping на7 ширинах; anchored editor+доступный save;403/429; единичные доступные modal notifications; мобильный UI password→TOTP/validation/back. После последних CSS правок проверены именно затронутые users/editor/notifications, без повторного полного matrix.
- Матрица: Overview, VPN, Users, Nodes, Routing, Security, Versions, Events, Backup × 320/375/390/430/768/1024/1280/1440/1600/1920/2560. Без горизонтального overflow документа; широкие технические таблицы могут иметь собственный scroll.
- Скриншоты390/1440 сохранены локально в `/private/tmp/arcnet-ux-review`; просмотрены Users, VPN, Settings и editor/error состояния. Это не утверждение о ручном визуальном просмотре всех99 комбинаций.
- `git diff --check` PASS. Go regression/race/release tests не запускались: серверные файлы не менялись. Запускался только Go browser fixture: после browser PASS и составления отчёта он достиг30-минутного лимита и завершился `FAIL: browser fixture timeout` (1800.36s). POST cleanup затем получил connection refused; `docker ps --filter name=arcnet-ux-review` подтвердил отсутствие контейнера. Это не Go test PASS и не ошибка, возникшая внутри прошедших браузерных сценариев; повтор ради зелёного fixture не выполнялся.

Начальные browser остановки были диагностическими: после injected409 требовалась изоляция query cache для следующего сценария; ожидание retry первоначально проверяло heading до окончания запроса; Field error менял label name. Причины исправлены, assertions не ослаблялись. Последний полный runtime результат сохранён отдельно от targeted результатов.

После последних уточнений отдельно PASS: повторный вход с invalidation существующих queries без потери draft; открытие details через имя и unknown создания подключения без вечного pending. Результаты находятся в `final-reauth/results.json` и `final-create/results.json` локального каталога artifacts; это два targeted запуска, не новый полный matrix.

### Воспроизведение и передача

```sh
npm ci --prefix frontend --no-audit --no-fund
npm run build --prefix frontend
npm test --prefix frontend
```

Поднять существующий `TestOnlineUpdateBrowserFixture` с `ARCVPN_ONLINE_UPDATE_BROWSER=1 ARCVPN_UI_REVIEW=1`, `go test -count=1 -v ./internal/httpapi -run '^TestOnlineUpdateBrowserFixture$' -timeout 31m`. Он слушает18394, ограничен30 минутами, завершение — `POST /fixture-done`. В данной среде использован уже установленный image `arcvpn-local-test:20261105`, read-only `/workspace`, отдельный `/tmp`, без запуска рабочего VPS. Это test fixture, не команда запуска production.

```sh
NODE_PATH=/private/tmp/arcvpn-browser/node_modules node tests/ui_ux_audit_e2e.mjs
NODE_PATH=/private/tmp/arcvpn-browser/node_modules node tests/ui_ux_layout_e2e.mjs
```

Требуется доступный Playwright; executable Chrome задаётся `ARCVPN_CHROMIUM`, local base URL — `ARCVPN_E2E_URL`, artifacts — `ARCVPN_E2E_OUTPUT`. Скрипты отвергают внешний hostname. Никаких настоящих административных credentials: только disposable fixture. Для точечного продолжения main suite допускает `ARCVPN_E2E_FIRST_STEP`/`ARCVPN_E2E_LAST_STEP`; такой запуск не считается полным suite PASS.

### Ограничения приёмки

NOT RUN: реальные iOS/Android keyboards/safe-area, VoiceOver/NVDA и независимый WCAG2.2 AA audit, Windows classic scrollbar, native browser200% zoom, большой production dataset, реальное выполнение SSH/UFW/update/restore/VPN на VPS. Внешние server/runtime acceptance gates остаются прежними. TOTP в этом UI-блоке проверялся управляемыми ответами, не настройкой второго фактора рабочего администратора. QR отрисовка/failure проверены, фактическое сканирование VPN-клиентом не выполнялось. Не заявлять отсутствие всех CSS/UX дефектов во всех возможных состояниях.

Статус: **frontend source/build/unit + перечисленные browser scenarios PASS**; это не полный independent accessibility audit и не новый установочный релиз. После pull следующая нейросеть должна читать этот отчёт и актуальный current-task; runtime/Geo/квоты не возобновлять автоматически в рамках UI-задания.

## Изменённые файлы этой итерации

```text
SOURCE_MANIFEST.sha256
docs/ai/change-log.md
docs/ai/current-task.md
docs/ai/index.md
docs/design/design-system.md
docs/design/ui-ux-audit-2026-11-09.md
docs/features/first-connection.md
docs/features/users-and-devices.md
frontend/src/app/Auth.tsx
frontend/src/app/Layout.tsx
frontend/src/features/connections/ConnectionDetails.tsx
frontend/src/features/connections/ConnectionEditor.tsx
frontend/src/features/connections/ConnectionForms.tsx
frontend/src/features/connections/ConnectionTable.tsx
frontend/src/features/connections/KeyRotationDialog.tsx
frontend/src/features/routing-profiles/RoutingProfiles.tsx
frontend/src/features/settings/UpdateSettings.tsx
frontend/src/features/users/AccountSettings.tsx
frontend/src/features/users/BulkCreateUsers.tsx
frontend/src/features/users/BulkUserAccess.tsx
frontend/src/features/users/DevicesDialog.tsx
frontend/src/features/users/DirectAccess.tsx
frontend/src/features/users/UserDetails.tsx
frontend/src/features/users/UserEditor.tsx
frontend/src/features/users/UserToolbar.tsx
frontend/src/main.tsx
frontend/src/pages/Backup.tsx
frontend/src/pages/Events.tsx
frontend/src/pages/Nodes.tsx
frontend/src/pages/Overview.tsx
frontend/src/pages/Routing.tsx
frontend/src/pages/Security.tsx
frontend/src/pages/Users.tsx
frontend/src/pages/VPN.tsx
frontend/src/shared/Confirmation.tsx
frontend/src/shared/Dialog.tsx
frontend/src/shared/EditorSections.tsx
frontend/src/shared/Feedback.tsx
frontend/src/shared/api.ts
frontend/src/shared/design-system/components.css
frontend/src/shared/design-system/tokens.css
frontend/src/shared/ui.tsx
frontend/src/style.css
frontend/tests/api-feedback.test.ts
tests/ui_ux_audit_e2e.mjs
tests/ui_ux_layout_e2e.mjs
```

Критерии сверены с [React state preservation](https://react.dev/learn/preserving-and-resetting-state), [Radix Dialog](https://www.radix-ui.com/primitives/docs/components/dialog) и [WAI APG Alert](https://www.w3.org/WAI/ARIA/apg/patterns/alert/). Skills Impeccable/Vercel/emilkowalski проверены на доступность; локально отсутствовали, не устанавливались. Проверки выполнены вручную и средствами проекта.
