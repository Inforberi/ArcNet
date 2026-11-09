# Пароли и менеджер версий — результат блока 9 ноября 2026

Начальная точка9b7e091. Пользователь заменил прежнее UI-only ограничение новым заданием: пароли8 Unicode code points, SemVer, история/обновления в Settings, простой выпуск с прежней Ed25519-моделью. VPN/DB/KDF/CSRF/2FA не менять. Скилл go-api-nethttp применяется. Не возобновлять квоты/Geo и прочий runtime backlog в этой задаче.

## Аудит и решения

- HashPassword/setup/change/reset расходились с Unicode; min16 был байтовым. Новая политика:8 code points, максимум256 UTF-8 bytes, без normalization/trim. CLI снимает только один транспортный LF/CRLF; новый пароль однострочный. VerifyPassword не получает ретроспективный минимум. Frontend Array.from + TextEncoder, surrogate errors; составные символы считаются отдельными code points, не graphemes.
- Существующий v1 catalog strict legacy stage5-aN; добавление SemVer/полей в него сломает A7–A9. Сохраняем неизменённый подписанный v1 и ключ. BridgeA10 с новым parser должен выйти через v1. Новые нормальные версии идут через v2 URL/подписанный v2 catalog. v2 ошибка подписи не скрывается fallback; недоступность транспорта допускает проверенный v1 с явной partial пометкой. До публикации v2 это нормально, не «всё актуально».
- releaseversion — SemVer precedence, numeric legacyA9<A10, legacy ниже новых релизов того же core. Metadata не создаёт второго public release; новый publish ID без `+`. Минимум и replay/downgrade перепроверяются privileged verifier.
- Старый backup21 не читается A7/A8; persisted JSON/таблицы/назначения/units/helper различаются. Нет tested DB/config restore path: ни один произвольный downgrade не разрешать. `.previous` — только компенсация binary/core-ui failure, не перенос данных назад.
- Каталог — источник доверия установки. GitHub Releases — отдельная bounded информационная история, не источник install URL/разрешения. Даты не выдумывать.
- UI: один Settings→Updates, `/versions` alias; current/recommended/staged/progress/history/advanced, точные IDs только в details. Unknown pending переживает reload, ожидание3мин bounded; не auto apply/retry.

## Уже изменённые исходники / проверки

Готово в исходниках: auth policy/backend/CLI и shared frontend policy; releaseversion/catalog v2/legacy parser; root verifier через общий updater; releasebundle SemVer; VERSION0.7.1 (ещё не опубликован); компактный Settings manager, alias, история, bounded persistent unknown; pure-Go `prepare-update` и trusted-host `scripts/release-update.sh`. [Команды выпуска и восстановление прерванной публикации](../ops/release-update.md).

Наблюдавшиеся проверки:

- Linux targeted race PASS: `go test -race ./internal/auth ./internal/releaseversion ./internal/updater ./internal/releasebundle ./cmd/arcvpn-release ./cmd/arcvpn ./internal/httpapi -run 'TestNewPassword|TestOrdering|Test.*Catalog|Test.*Update|TestPrivilegedInstaller|TestReleasePreparation|TestHistory|TestResetAdministrator|TestAdminRegistration' -timeout 4m`, с `ARCVPN_TEST_RELEASE_CORE` на собственный ранее проверенный Linux ELF. Auth8/Unicode/legacy login, setup/change/MFA/session revoke, CLI recovery, v2/signature/fallback, history/no downgrade и package/signing covered. В race-фильтре releasebundle был `[no tests to run]`.
- Отдельно полные `go test ./internal/releasebundle ./internal/releaseversion ./internal/updater` PASS. `go vet` семи изменённых Go-пакетов PASS. После исправления legacy build metadata и добавления отказов invalid build receipt повторены `go test ./internal/releaseversion ./internal/updater ./cmd/arcvpn-release`: PASS. Остальные inputs не менялись, race не дублировался.
- Frontend37/37 tests PASS, TypeScript/Vite production build PASS. После последних эквивалентных validation-call правок build повторён и PASS.
- `tests/version_manager_e2e.mjs`:8 сценариев PASS,10 widths320/375/390/430/768/1024/1280/1440/1600/1920 без document overflow,0 frontend errors. Локальные React + Go/SQLite, настоящий тестовый Ed25519 catalog/ZIP/download/stage/discard/apply request; systemd helper имитирован. Restart success/failure/unknown, GitHub metadata/error states и account submit перехвачены Playwright, не внешняя сеть и не VPS. Проверены exact version request, double-click single download/apply, confirmation cancel, readonly signed-old/unsigned-history, persisted timeout/reload, stale result distinction, helper/catalog failure/retry,4emoji rejection и8 Unicode со значимыми пробелами. Setup8 backend integration PASS; отдельный браузерный setup не запускался.
- `bash -n` четырёх build/release scripts PASS; `release-update.sh --help` PASS; запуск из dirty snapshot безопасно отказал exit65 до сборки/подписи/публикации. Go prepare tool проверен на подписании ephemeral fixture key, VerifyPublished, повторном output, чужом ключе, неверной версии/receipt/hash/platform/frontend flag. Реальный pinned signer не подменялся.
- Браузерный стенд завершён POST `/fixture-done`, Go fixture PASS. Логи: `/private/tmp/arcnet-version-{race,vet,final-go,frontend-tests,frontend-build,browser,browser-fixture}.log`, screenshots/results `/private/tmp/arcnet-version-review`. Временные логи не добавляются в Git.

Новый полный installer build из этого source snapshot, реальная подпись A10/0.7.1, GitHub publication, настоящий systemd apply/rollback и VPS/VPN acceptance **не выполнены**. Build pipeline изменён для receipt и Python3 только на builder; полный pipeline выполняется командой выпуска на машине с ключом. Fixture receipt проверяет связку builder/SHA, не является независимой аттестацией. Нельзя считать старый unsignedA10 текущей сборкой.

## Выпуск и ограничение ключа

Перед новым заданием Linux/amd64 A10 был успешно собран из чистого9b7e091, `dist/installer-0.7.0-stage5-a10`, log `/private/tmp/arcnet-a10-build.log`; версия не подписана и не опубликована. Этот артефакт не содержит нынешние изменения. Номер0.7.1 — новый планируемый normal release; bridge по-прежнему нужен старым клиентам.

Доверенный public key SHA256 `5f80f78a69441940b93bece69ce027f4ffe4f26452f5bed96da8bb520b4cd69a`. На этом компьютере только другой private key, пользователь подтвердил: правильный ключ на другой машине. Никакой ротации/обхода trust. Упростить выпуск до команды на машине с ключом; ключ вне Git/Docker/VPS. Готовый релиз не обещать до подписи и публикации.

Следующий шаг на другой машине: получить этот source commit, запустить `release-update.sh --bridge --publish` с существующим ключом, установить A10 на тестовую старую панель, затем `release-update.sh --publish` для0.7.1. Инструкция выше содержит точные команды, зависимости и восстановление частичного выпуска. Ни один произвольный downgrade не разрешён. Остальной runtime backlog этой задачей не закрыт.

Референсы изучены: [SemVer2](https://semver.org/spec/v2.0.0.html), [3x-ui x-ui.sh](https://github.com/MHSanaei/3x-ui/blob/master/x-ui.sh), `legacy_version()`/`update()`. В upstream есть выбор тега через shell installer и eval; эти механизмы не переносим в ArcNet. Разрешение signed catalog + platform + manifest сохраняется.
