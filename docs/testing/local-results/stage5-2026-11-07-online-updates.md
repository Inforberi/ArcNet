# A7 — онлайн-обновления: source/browser

2026-11-07. [Контракт](../../features/online-updates.md). Только core/UI, без изменения схемы/units/SSH/firewall. Новый updater — не полная миграция произвольного релиза.

## Выполнено

- Linux existing image `arcnet-remote-tests:20261106`, network none: `go test -race ./internal/updater ./internal/httpapi ./cmd/arcvpn -run 'TestSignedUpdateCatalog|TestUpdateCatalog|TestUpdateRedirect|TestUpdateHelper|TestOnlineUpdate|TestInstallerRollback|TestPrivilegedInstaller' -count=1` — PASS (3.165s/1.130s/1.026s).
- Подпись/изменение payload/чужой ключ, platform/minimum-version/downgrade/replay, неправильный ZIP/hash/version, unsafe metadata/redirect — PASS. Root verifier проверяется отдельно от HTTP.
- HTTP auth/CSRF, ограничения concurrency/каталога, отмена и повторная загрузка, сохранение подготовленного пакета при плохом upload, ожидаемая версия, busy/failure state — PASS. Systemd заменён явным Unix helper fixture; это не настоящий systemd.
- Node22 `npm run build` — TypeScript/Vite PASS.
- CUA browser + настоящий React/HTTP/SQLite test stand: вход → Updates → synthetic A8 → серверная загрузка → проверенный пакет/доступная установка; mobile390 без горизонтального overflow. Стенд остановлен. После наблюдения исправлены stale «Загружаем…» и формат маленького размера, TS/Vite повторён PASS. Установка из браузера не запускалась.

## Не выполнено

Настоящий VPS/systemd apply/rollback/перезапуск helper, сетевые отказы CDN, обновление на сервере пользователя и VPN handshake — NOT RUN. Старый A5/A6 helper не dispatch-ил update, поэтому нужен первый переход существующей update unit через Termius. Ключ доверия и проверка подписи не обходятся. Новые версии после A7 должны сначала публиковаться и включаться издателем в подписанный каталог.

Release pipeline и точные опубликованные sha/source revision записываются после сборки в Release/BUILDINFO; приведённые source/browser результаты не подменяют эти gates.
