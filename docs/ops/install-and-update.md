# Установка, обновление и выпуск ArcVPN

Это руководство для нового администратора и сопровождающего. Текущая версия — 0.7 preview; systemd/ACME/VPN/полный update на VPS ещё требуют внешней приёмки. Публичный репозиторий: [Inforberi/ArcNet](https://github.com/Inforberi/ArcNet), лицензия проекта — [MIT](../../LICENSE).

Пользователь проведёт серверные испытания самостоятельно. A1 CLI и A2 локальный подписанный Linux x86_64 пакет подготовлены: [release/migration guide](https://github.com/Inforberi/ArcNet/blob/main/docs/ops/installer-release-and-migration.md). Нужны core/wrapper/units/tmpfiles/hook вместе; core-only updater не мигрирует обвязку. Опубликованы тестовый prerelease и A3 bootstrap: [одна команда и короткая приёмка](vps-preview-acceptance.md). Systemd/VPS ещё не приняты.

## Что скачать

Тестовый [GitHub Release 0.7.0-stage5-a3](https://github.com/Inforberi/ArcNet/releases/tag/v0.7.0-stage5-a3) содержит подписанный installer, закреплённый bootstrap и точный source snapshot сборки. Используйте [проверенную установочную команду](vps-preview-acceptance.md); source ZIP не заменяет installer.

Пакет содержит core со встроенным React, `install.sh`, требуемые `deploy/`-скрипты, README/лицензию/руководство, `dist/VERSION` и `dist/SHA256SUMS`. Приватные ключи подписи, SQLite, пользовательские данные и случайные файлы `dist` в него не включаются. Стандартная сборка не требует AmneziaWG.

## Сборка пакета сопровождающим

На отдельной Linux x86_64 машине: Go 1.26+, Node.js 22.12+, npm, GCC, GNU tar/coreutils, SQLite headers и qrencode runtime. Для Debian/Ubuntu системные build dependencies: `build-essential libsqlite3-dev libqrencode4`. Версии Go/Node устанавливает сопровождающий по доверенному процессу своей машины.

Проверенный рецепт A2: Ubuntu22.04 + Go1.26.6/Node22.21.1, опционально через `./scripts/build-installer-docker.sh`. Native builder требует актуальный SOURCE_MANIFEST.sha256 и проверяет его перед упаковкой; BUILDINFO показывает revision/dirty/source hash. Для подписи используйте offline `arcvpn-release`, а не приватный ключ на VPS. [Подробности](https://github.com/Inforberi/ArcNet/blob/main/docs/ops/installer-release-and-migration.md).

CGO-бинарник зависит от системных библиотек. Собирайте на самой старой целевой ОС и проверяйте на каждой заявленной платформе; сборка на новой ОС не доказывает ABI-совместимость со всеми старыми VPS.

```bash
git clone https://github.com/Inforberi/ArcNet.git
cd ArcNet
ARCVPN_BUILD_VERSION=0.7.0-stage5 ./scripts/build-release.sh
```

Скрипт проверяет committed lockfile, собирает React/core вместе, выполняет release checks и создаёт установочный tar.gz. Это явная release-операция, она может быть долгой; не запускать её для проверки README. `scripts/package-release.sh` только упаковывает уже проверенные artifacts, не доказывает работоспособность бинарника. Свежая версия панели 0.7 не меняет автоматически ограничения подписанных stage4 node bundles 0.6.x; их выпуск/приёмка — отдельный процесс.

## Чистый VPS

Поддержанный установщиком target: Debian 12+ / Ubuntu 22.04+, x86_64, запущенный systemd, root/sudo, публичный IPv4. Нужны доступный снаружи TCP/80 для IP ACME, выбранный TCP-порт панели (8443 по умолчанию) и порты VPN. Не устанавливайте поверх действующей панели; сохраните SSH/консоль и snapshot тестовой VM. RAM/максимум пользователей пока не подтверждены измерениями.

Используйте [A3 bootstrap](vps-preview-acceptance.md): он проверяет подпись до безопасной распаковки. Ручной запуск install.sh допустим только из уже аутентифицированного payload. По умолчанию IP определяется автоматически. При необходимости до запуска можно задать:

```bash
sudo env ARCVPN_PUBLIC_IP=203.0.113.10 ARCVPN_PANEL_PORT=8443 bash install.sh
```

`203.0.113.10` — пример: замените на настоящий глобальный IPv4 VPS. Установщик проверяет занятые порты и существующую установку, создаёт `/opt/arcvpn`, `/var/lib/arcvpn`, `/etc/arcvpn` и systemd units, устанавливает runtime dependencies и получает IP-сертификат. Служба панели — `arcvpn-panel.service`. Xray/Hysteria скачиваются с проверкой upstream digest; WireGuard tools устанавливаются из пакетов ОС. Выключенный UFW не включается автоматически, SSH policy не заменяется. Облачный firewall провайдера настраивается отдельно.

Сохраните показанные учётные данные. Войдите по HTTPS, включите 2FA, создайте подключение и пользователя. Проверяйте трафик независимым клиентом: статус службы/heartbeat не доказывает VPN. Перед выдачей доступа другим людям проверьте egress, subscription, расход, срок, отзыв, перезапуск и восстановление. WireGuard выдаётся устройству отдельным `.conf`.

TCP REALITY/XHTTP/Hysteria2/WireGuard можно создать первым: отдельная настройка TCP REALITY не требуется. Адрес задаётся первым сохранённым профилем и должен совпадать у остальных. Hysteria2 требует установленного сертификата того же IP. Backup v11 переносит состояние без TCP REALITY; старый core такой архив не читает. [Подробности](../adr/012-independent-first-local-protocol.md).

## Обновление существующей установки

1. Прочтите release notes и ограничения совместимости; создайте согласованный снимок VPS/установки и зашифрованный portable backup вне сервера. Portable backup не включает админские сессии/все секреты ОС и не заменяет полный snapshot.
2. Используйте `update.zip`, подписанный доверенным Ed25519 ключом. Ключ проверки установлен в `/etc/arcvpn/update-public.hex`; его происхождение проверьте независимо от загружаемого обновления. Если ключ не установлен, signed updates недоступны — это явно показывается в UI.
3. В панели **Безопасность → Обновления** загрузите archive, проверьте подтверждённую версию и запустите применение. Приём запроса означает начало, не успешное завершение обновления.
4. После reconnect проверьте `sudo arcvpn status`, `sudo /opt/arcvpn/bin/arcvpn-core version`, вход в UI и реальные клиентские подключения. При ошибке смотрите `sudo arcvpn logs panel` и systemd журнал update service.

Updater перепроверяет подпись/размер/hash привилегированно, блокирует одновременный update, заменяет core и явно включённые engine binaries, перезапускает и проверяет службы. При ошибке пытается вернуть предыдущие бинарники; ошибка восстановления не скрывается. Проверка active службы не подтверждает весь VPN.

**Граница текущего updater:** shell wrappers, systemd units, конфигурация ОС и база не откатываются одним откатом бинарника и автоматически не мигрируют. Для релиза с такими изменениями нужна отдельная проверенная инструкция миграции. Новый root helper WireGuard/его stamp требует отдельной VM-проверки. Не запускайте `install.sh` повторно поверх существующей установки и не заменяйте рабочий core неподписанным файлом вручную.

## Подготовка signed update сопровождающим

После проверенной сборки на доверенной Linux-машине один раз создайте ключи **вне репозитория**:

```bash
arcvpn_keys_dir="$HOME/.local/share/arcvpn-signing"
./dist/arcvpn-linux-amd64 update-keygen --out-dir "$arcvpn_keys_dir"
install -m 0644 "$arcvpn_keys_dir/update-public.hex" dist/update-public.hex
./scripts/package-release.sh
```

Keygen не перезаписывает существующие ключи; повторно для следующих релизов его не выполняйте. Приватный ключ храните вне VPS и не публикуйте. Публичный ключ/его fingerprint передайте администраторам доверенным способом. Повторная упаковка добавит публичный ключ в installer bundle и integrity manifest.

```bash
./dist/arcvpn-linux-amd64 update-sign \
  --binary dist/arcvpn-linux-amd64 \
  --private-key "$arcvpn_keys_dir/update-private.hex" \
  --version 0.7.0-stage5 \
  --output dist/update.zip
```

Это core-only update. Флаги `--xray`/`--hysteria` используются только для явно проверенных engine binaries. Подпись не добавляет отсутствующие migration/VM tests. Публиковать готовый релиз следует только после отдельной приёмки; сборка пакета в этом репозитории сама ничего не публикует.

## Восстановление доступа

`sudo arcvpn settings` показывает адрес/логин без секретов. `sudo arcvpn password reset` меняет пароль, отзывает сессии и сохраняет 2FA; утрата 2FA требует явного `--reset-2fa`. [Подробное recovery руководство](local-admin-recovery.md). При восстановлении backup v11 уже известные расходы и локальные отзывы не отменяются. На новом IP старые ссылки сами не перенаправляются.
