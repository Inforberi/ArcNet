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

## Release A7

Snapshot `b4406a79e72972041272a7993617311dfe845b0e`; Linux/amd64 Go1.26.6/Node22.21.1 npm ci/TS/Vite, go test ./..., go vet ./..., embedded/version/lifecycle/package PASS. В full suite обновлён старый stage3 fixture: недоступный helper теперь даёт 409 раньше проверки тела; отдельный trusted-helper test сохраняет 422 для unsigned ZIP и 400 для unconfirmed apply.

Installer signature verifier и safe extraction bootstrap verify-only в Linux PASS (macOS не имеет обязательного timeout). Независимый Node crypto: ZIP signature/version/core hash и signed catalog/ZIP size/hash PASS. update-a7.zip SHA256 `92bb14d969c5c4eacd900086c8a63f8d8c59e828e2a0e7c06219844ee036abfb`, 4542753 bytes. Initial transition script SHA256 `9fa57d32fe3fdf9ff60cb0291dad0f8ecaad51bb53e243a4793ab75a3b252521`; bash -n PASS, реальное применение NOT RUN. BUILDINFO dirty=true из-за исключённой .serena/; точный snapshot/archive приложен к Release. Каталог с A7 опубликован отдельно после доступности пакета; в build snapshot он содержал только A6.
