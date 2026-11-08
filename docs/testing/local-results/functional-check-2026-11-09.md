# Проверка текущей функциональности — 9 ноября 2026

Проверена рабочая копия без commit/push/release/VPS изменений. Код во время проверки не менялся, новые тесты не писались. SHA256 выбранных исходников/конфигурации до и после совпал: `e49901b7c648908801af0333b88d531a75987c7aba8896f54200b9e77bd7de47`. Список: git tracked+untracked Go/TS/TSX/CSS/JSON/shell и go.mod/go.sum, без docs. Это fingerprint входов проверки, не подписанный release artifact.

## Фактические проверки

- Linux Docker, существующий образ arcnet-remote-tests:20261106, bind mount проекта и прежний Go cache. Команда: `go test -json ./... -count=1 -timeout 90s`. Exit0: 30 пакетов PASS; 3 пакета без тестов. 438 test/subtest PASS, 10 SKIP, FAIL0. Test/subtest count включает родительские тесты и подслучаи, не438 независимых пользовательских сценариев.
- `npm --prefix frontend run build`: TypeScript/Vite PASS,228 modules.
- `npm --prefix frontend test`:30 PASS,0 FAIL,0 SKIP.
- `git diff --check`: PASS.

Полный suite включает имеющиеся проверки auth/session/MFA/CSRF, хранения, пользователей/пакетов/расписаний/учёта, профилей/экспорта, renderer/apply/компенсации, backup, подписанного updater, нод и control-plane маршрутов. Наличие package PASS не означает, что новая UI-ветка или все комбинации параметров покрыты. Для новых async confirmations/editor sections/duplicate inbounds отдельной полной браузерной регрессии нет.

## Живой локальный API

Использован только disposable fixture127.0.0.1:18394 с synthetic credentials, mock Xray и SkipRestart. Серверные ключи/подписки/ответы с credentials не выводились.

- unauthenticated access: blocked
- login/session: PASS
- /api/users: PASS
- /api/users/usage: PASS
- /api/vpn: PASS
- /api/protocols: PASS
- /api/protocols/health: PASS
- /api/overview: PASS
- /api/settings/monitoring: PASS
- /api/settings/logs: PASS
- /api/events: PASS
- /api/system/update: PASS
- create temporary user: PASS
- CSRF missing: blocked
- atomic editor save: PASS
- editor persistence: PASS
- subscription export: PASS
- QR SVG: PASS
- temporary user cleanup: PASS

Редактирование проверило name/title/group/явные разрешённые подключения через атомарный editor endpoint и повторное чтение. QR проверен как HTTP200 SVG, не сканирован камерой. Проверка подписки подтверждает формирование экспорта, не VPN handshake. DELETE удалил только созданного проверкой пользователя. Исходный Browser user не редактировался; в тестовом журнале могли добавиться события проверки.

## Пропуски

- `TestRealHysteriaAccountingAndRevokedDevice`
- `TestSignedNodeReleaseRoundTripAndTamperResistance`
- `TestOnboardingBrowserFixture`
- `TestCascadeUIBrowserFixture`
- `TestDevicesBrowserFixture`
- `TestFirstProtocolBrowserFixture`
- `TestRealRealityConfiguration`
- `TestRealXrayStatistics`
- `TestRealCascadeForwardingAndClosedFailure`
- `TestOnlineUpdateBrowserFixture`

Эти opt-in/real-engine сценарии не запущены обычным suite и не засчитаны как PASS. Browser fixture online update отдельно работает как preview, но его lifecycle не завершён как тест и это не E2E приёмка.

Не проверены: реальный браузер/keyboard/focus/mobile/визуальная геометрия и сканирование QR (local browser use запрещён Work settings), реальный VLESS/XHTTP/Hysteria2/WireGuard трафик, kernel/network/NAT/UFW и смена портов, renew сертификата, reboot, реальное восстановление/обновление VPS, удалённые ноды/каскадный handshake. Опасные network проверки требуют отдельной изолированной VM и rollback по правилам проекта. Существующий VPS с3x-ui не затрагивался.

## Вывод

В выполненном локальном suite и выбранном live API сценарии ошибок не найдено. Утверждения «вся функциональность работает», «сервер максимально защищён» и production-ready из этих результатов не следуют. Открытый backlog runtime/advanced fields/IP enforcement/full updater сохраняется в текущем плане.
