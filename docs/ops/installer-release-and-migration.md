# Установочный пакет A3 и перенос старой установки

2026-11-06. Подготовлен локальный **тестовый** Linux x86_64 пакет со встроенным UI, build metadata и подписью всего tar.gz. Опубликован [тестовый prerelease](https://github.com/Inforberi/ArcNet/releases/tag/v0.7.0-stage5-a3); [A3 bootstrap и инструкция приёмки](vps-preview-acceptance.md) закрепляют версию/ключ и проверяют пакет до исполнения. Реальные systemd/ACME/VPN/reboot ещё проверит пользователь.

## Что подготовлено

- `scripts/build-installer-docker.sh`: optional build-host Docker/BuildKit, target linux/amd64, Ubuntu22.04, Go1.26.6 и Node22.21.1. На VPS Docker/Go/Node не нужны.
- Native release script остаётся доступным на Linux x86_64. CI baseline изменён с Ubuntu24.04 на22.04, чтобы не строить CGO для более новой ОС, чем допускает installer. Это не подтверждение всех производных ОС/старых CPU/kernel.
- Installer tar.gz: core, install.sh, wrappers/helpers, документация, VERSION, BUILDINFO, SOURCE_MANIFEST и внутренний SHA256SUMS. Xray/Hysteria и системные runtime dependencies установщик получает отдельно; пакет не является offline bundle.
- BUILDINFO фиксирует toolchain, build OS, source commit, dirty flag и hash исходного manifest. Экспортируется полный source tar именно проверенного build snapshot. `source_dirty:true` явно обозначает снимок рабочего дерева; это не утверждение, что артефакт равен чистому Git commit.
- Рецепт повторяем, но byte-for-byte reproducibility не заявлена: registry/apt snapshots и tar timestamps отдельно не фиксировались. Digest метаданных Docker сохраняется рядом с build log; никаких приватных signing keys внутри builder нет.

```bash
./scripts/build-installer-docker.sh
```

Результат по умолчанию: `dist/installer-0.7.0-stage5-a3/`. Повторять full build для одной правки документации не требуется. Подписанный локальный артефакт соответствует своему экспортированному source manifest; последующие docs-only commits не объявляются повторно собранным бинарником.

## Подпись и доверие

`arcvpn-release` — отдельный pure-Go offline tool машины сборки, не часть VPS runtime. Сначала собрать его из доверенных исходников:

```bash
CGO_ENABLED=0 go build -o dist/arcvpn-release ./cmd/arcvpn-release
./dist/arcvpn-release keygen --out-dir "$HOME/.local/share/arcvpn-installer-signing-preview-a3"
```

Keygen выполняется **один раз**, существующие ключи не перезаписываются. Каталог0700, private key0600; не копировать private key в Git, Docker context или на VPS. Сохраните его защищённую резервную копию. Для следующих выпусков используйте тот же выбранный operator key или отдельную документированную процедуру ротации.

```bash
arcvpn_release_dir=dist/installer-0.7.0-stage5-a3
arcvpn_archive="$arcvpn_release_dir/arcvpn-0.7.0-stage5-a3-linux-amd64.tar.gz"
./dist/arcvpn-release sign \
  --archive "$arcvpn_archive" --version 0.7.0-stage5-a3 \
  --source-manifest "$arcvpn_release_dir/SOURCE_MANIFEST.sha256" \
  --manifest "$arcvpn_archive.manifest.json" --signature "$arcvpn_archive.manifest.sig" \
  --key "$HOME/.local/share/arcvpn-installer-signing-preview-a3/installer-private.hex"
./dist/arcvpn-release verify \
  --archive "$arcvpn_archive" \
  --manifest "$arcvpn_archive.manifest.json" --signature "$arcvpn_archive.manifest.sig" \
  --key "$HOME/.local/share/arcvpn-installer-signing-preview-a3/installer-public.hex"
```

Sidecars создаются exclusive, без молчаливого перезаписывания. Для нового содержимого выпускайте новую выбранную версию/новый контролируемый набор файлов. Verify проверяет подпись, metadata, имя/размер и полный hash архива без его исполнения/распаковки. Подпись относится к `SignatureDomain || manifest-json-bytes`, `.sig` — raw64 bytes. Public hex32 bytes/PEM Ed25519 и fingerprint получаются от владельца доверенным способом, а не только рядом с тем же недоверенным архивом. Приватный ключ updater и новый installer signer не объединяются автоматически.

Текущий local operator signer fingerprint (SHA256 raw public key):

```text
5f80f78a69441940b93bece69ce027f4ffe4f26452f5bed96da8bb520b4cd69a
```

Это новый отдельный ключ A3; прежний private key A2 на машине недоступен, поэтому существующие установки не получают автоматическую ротацию доверия. Это локально созданный ключ предварительного пакета, не внешняя аттестация издателя. A3 закрепляет этот public key внутри bootstrap; доверенная установочная команда отдельно закрепляет SHA256 самого bootstrap. Непроверенный скачанный core/verifier запускать от root для проверки самого себя нельзя.

## Старый сервер: безопасный перенос всех компонентов

Автоматическая in-place миграция wrappers/units/tmpfiles/cert hook и OS настроек **не реализована**. `update.zip` меняет core/явно выбранные binaries; повторный install.sh поверх старой панели запрещён. Для текущей preview-приёмки поддерживаемый маршрут обновления всей обвязки — **чистая тестовая установка + portable restore**:

1. На старом VPS сохранить off-server snapshot ОС/диска и зашифрованный VPN backup. Убедиться, что восстановление не зависит от единственной работающей панели. Секреты TLS/SSH и админский доступ не переносятся одним portable backup автоматически.
2. На отдельном чистом целевом VPS установить проверенный полный A3 пакет по [процедуре](install-and-update.md#чистый-vps). Не выключать старый сервер до проверки нового.
3. Создать новый административный доступ/2FA, восстановить переносимый VPN backup с preview. v1–v11 читаются текущим core, известные расходы/отзывы не уменьшаются. При новом IP выпустить собственный сертификат; прежние ссылки не начинают указывать на новый сервер сами.
4. Проверить реальные подключения, устройства/отзыв, пакеты, backup/restore, root CLI, renewal и reboot по [плану A4](https://github.com/Inforberi/ArcNet/blob/main/docs/planning/pre-vps-install-and-cli-2026-11-06.md). Подтвердить desired/applied, не только active unit.
5. После успешной проверки переключить клиентские subscription URLs/домен и наблюдать результат. Старую установку сохранять для rollback, пока новый путь не принят. При возврате учесть новые списания/отзывы: старый snapshot не должен молча вернуть уже отозванный доступ.

Если второго VPS нет, нужна изолированная VM/полный проверяемый snapshot с остановкой и восстановлением ОС; отсутствие такого плана не заменяется перезаписью working core. Будущий in-place migrator должен отдельно учитывать core/wrapper/helpers/units/tmpfiles/journald/cert hook, desired enable-state, БД и rollback каждой изменённой сущности. Нельзя назвать такую миграцию готовой до испытаний.

Фактическая сборка/подпись: [результаты](../testing/local-results/stage5-2026-11-06-installer-bundle.md). A3 реализован: [одна команда / приёмка](vps-preview-acceptance.md). Локальная проверка bootstrap не подтверждает установку на VPS.
