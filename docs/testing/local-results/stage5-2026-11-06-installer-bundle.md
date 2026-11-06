## Новый preview A3 — 2026-11-06

- `ARCVPN_BUILD_VERSION=0.7.0-stage5-a3 ./scripts/build-installer-docker.sh` — **PASS**: npm ci, TypeScript/Vite, Go test ./..., Go vet ./..., bash syntax, CGO Linux/amd64 binary, frontend-check, version/lifecycle contract и packaging. Отдельный full race не запускался.
- `bash deploy/bootstrap-install.sh --verify-only --from-dir dist/installer-0.7.0-stage5-a3` — **PASS** на настоящем архиве в Linux. Существующий `python3 scripts/testing/bootstrap-fixture.py` — **PASS**: valid, traversal/symlink/hardlink/duplicate/setuid/checksum-path/tamper/bad-signature; payload не исполнялся.
- UI каскадов — **PASS**: [целевой браузерный сценарий](stage5-2026-11-06-cascade-ui.md).
- `arcvpn-release sign` и `verify` настоящего архива — **PASS**. Новый installer signer A3: `5f80f78a69441940b93bece69ce027f4ffe4f26452f5bed96da8bb520b4cd69a`; private key вне Git/Docker/VPS. Это отдельное доверие для чистой установки, не автоматическая ротация A2.
- Artifact SHA256: `1549a9604c0bdb339748f08b88d3d4b29d0eb691530d8bb8e6bcf5036ddbc7b3`; source manifest SHA256: `d5375916094f39edefa33af9799bc3f5f4a3472b2bc755deb601e747f1ecf30d`. BUILDINFO: исходный commit `1ab39bbf5c9be968305129892318ab587faf7e45`, dirty=true, Go1.26.6, Node22.21.1, Ubuntu22.04. Точный исходный snapshot приложен к релизу; последующее обновление документации не меняет бинарник.
- Первая сборка остановилась на полном Docker-диске; пользователь разрешил удалить неиспользуемый build cache (освобождено11,58ГБ). Проверки подписей apt не отключались. После освобождения места сборка прошла.

**NOT RUN:** установка на VPS, systemd/ACME/VPN handshake, фактический remote revoke/расход, reboot/restore и полный updater. Пользователь выполняет серверную приёмку самостоятельно по [инструкции](../../ops/vps-preview-acceptance.md). A3 — preview, не production acceptance. История A2 ниже.

# A2 — первый Linux x86_64 installer, 2026-11-06

**BUILD_PASS / LOCAL_SIGNATURE_PASS / VPS_NOT_RUN**. Это локальный тестовый пакет, не опубликованный GitHub Release. [Контракт](../../ops/installer-release-and-migration.md).

## Единственный полный release-прогон

```sh
./scripts/build-installer-docker.sh
```

Exit0. Docker/BuildKit, linux/amd64 через Docker Desktop на arm64 host, builder Ubuntu22.04. Go1.26.6, Node22.21.1, системные SQLite/libqrencode4 из build baseline. Одноразовый release pipeline выполнил npm ci → TypeScript/Vite → go test ./... → go vet ./... → embedded core build → frontend-check/version/capability → package. Это первый прогон на новом целевом build OS/architecture, не повторный неизменённый race/browser matrix. Frontend140 modules; HTTP tests32.952s, releasebundle0.060s; весь release RUN203.2s после подготовки образов.

Результаты: `embedded React frontend: OK`, `arcvpn 0.7.0-stage5-a2`, `local management protocol: lifecycle-v1`. Runtime container/systemd/VPN не запускались.

## Снимок и artifacts

Каталог `dist/installer-0.7.0-stage5-a2/`:

- `arcvpn-0.7.0-stage5-a2-linux-amd64.tar.gz` — около4.1MiB, compiled core/UI/installer/wrappers/docs/metadata/integrity manifest.
- `.tar.gz.sha256`, `.tar.gz.manifest.json`, `.tar.gz.manifest.sig` — hash и подпись полного архива.
- `arcvpn-0.7.0-stage5-a2-source.tar.gz`, `SOURCE_MANIFEST.sha256`, `BUILDINFO.json`, `VERSION` — точный build snapshot/provenance. Public hex/PEM добавлены рядом отдельно, private key не попадает в archive/context/Git.

BUILDINFO: base commit `f3946a8863f46a54f63b9a4e8cfebbf807cc365e`, **source_dirty=true**, source manifest SHA256 `2c0ccc65b72b8f14079842fc6263a2ada01fdf80f9035f6108a8ed7c7149fc82`. Артефакт построен из рабочего снимка A2 до финальных documentation/CI commits; старый commit не выдаётся за его единственное содержимое. Source tar/manifest позволяют проверить этот снимок. Последующие изменения — native dirty-detection/CI baseline и документация; бинарник не объявляется пересобранным из этих изменений.

## Подпись и отрицательные случаи

Pure-Go `cmd/arcvpn-release` собран на host, private key создан один раз вне Git: `$HOME/.local/share/arcvpn-installer-signing/installer-private.hex`0600. Keygen не печатал key material. Sign и verify настоящего4.1MiB артефакта — exit0. Signer fingerprint SHA256 raw public key: `3f0f85ed250a916bfdeefd78d8eb15f84b90e23f67939ae3b3c25ac62f933c24`.

Новые `internal/releasebundle` tests вошли в единственный go test release-прогон: wrong signer, изменённый архив/manifest, unsafe version, отделение signature domain от updater, сохранность существующего keypair, mode private key и symlink rejection — PASS. File reads/stream hash ограничены по размеру и проверяют regular/no-follow. Private key остаётся только на машине оператора; прикладываемый public key не объявляется независимым trust anchor.

`bash -n scripts/build-release.sh scripts/package-release.sh scripts/build-installer-docker.sh tests/release_packaging.sh` и изменённый `bash tests/release_packaging.sh` в одноразовом Linux — exit0/PASS: BUILDINFO/source manifest включены, private/stale artifacts исключены, tamper отвергнут. Полные Go/frontend/race/E2E повторно не запускались после docs/CI/native metadata правок; новый GitHub Actions прогон не заявляется пройденным.

## Что ещё не подтверждено

Настоящая установка/запуск A1 CLI, systemd/ACME/WG/NAT/SSH, независимый VPN handshake, reboot/rollback и512МиБ/1vCPU — **NOT RUN**. Debian12 binary runtime отдельно не запускался; полный in-place migrator отсутствует. Не заявляются byte-for-byte builds, публичная публикация, готовый сетевой bootstrap или гарантия всех ABI/kernel/CPU. Следующий A3 требует отдельно проверить trust/pinned key и safe extraction до root execute.
