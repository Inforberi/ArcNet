# Обновление тестового стенда A4 → A5

Для существующего стенда с работающей A4 и сохранённой 3x-ui. Выполняется в root-shell Termius. Installer повторно не запускается. Пакет: [A5 prerelease](https://github.com/Inforberi/ArcNet/releases/tag/v0.7.0-stage5-a5), исходники core — `c320b03b6ceda48d236fa5d2feb1ce2280c8fe47`, точный source manifest/BUILDINFO приложены к релизу.

Обновляются core со встроенным UI и терминальное меню. Аккаунт, VPN-данные, движки, TLS, units, SSH и 3x-ui не заменяются. В этом блоке схема БД не менялась. На чистой A5 используется одноразовый setup, существующий администратор A4 сохраняется; свой логин/пароль задаётся в «Безопасность».

На стенде нет `/etc/arcvpn/update-public.hex`. Команда ниже устанавливает публичный ключ той же preview signing identity, что закреплена в bootstrap A3–A5. Это постоянное доверие подписанным этим ключом обновлениям, не SSH-ключ. Уже существующий отличный ключ не перезаписывается. Приватный ключ на сервер не передаётся. Проверка SHA256 здесь закреплена независимо от загружаемых файлов; штатный root updater дополнительно проверяет Ed25519 и hash core.

Опубликованная оболочка этого же блока: `update-a4-to-a5.sh`, SHA256 `27bd65b8927657dc6726e07a11a52bb776482a5102c4ad795d4b721ecb2ca74f`. Она дополнительно требует root и `set -euo pipefail`; скачать из A5 release, проверить этот hash до выполнения.

Штатный updater сохраняет core в `.previous` и пытается откатить его при отказе запуска панели. Меню заменяется атомарно только после успешного core update; его прежняя копия остаётся в закрытом временном каталоге. Это не полный updater ОС/units и не гарантия отката любой будущей миграции БД. Для важных данных нужен snapshot VPS до обновления.

```bash
(
set -e
umask 077
test "$(/opt/arcvpn/bin/arcvpn-core version)" = 'arcvpn 0.7.0-stage5-a4'
test ! -L /usr/local/bin/arcvpn
test ! -e /var/lib/arcvpn/update/staged.zip
test ! -L /var/lib/arcvpn/update/staged.zip
d=$(mktemp -d /var/tmp/arcvpn-a5-update.XXXXXXXX)
base=https://github.com/Inforberi/ArcNet/releases/download/v0.7.0-stage5-a5
curl -fLsS --connect-timeout 15 --max-time 120 "$base/update-a5.zip" -o "$d/update.zip"
curl -fLsS --connect-timeout 15 --max-time 60 "$base/arcvpn-menu-a5.sh" -o "$d/menu.sh"
printf '%s  %s\n' 128c9f0d9bc98cafe7490b1d25156a8ee975f3377c722e45930ae04a8da2e2fe "$d/update.zip" 5fd6c75edf56d1e32a0607e060f3733e121acbf6a30f1eebb48194b9e3fe2593 "$d/menu.sh" | sha256sum -c -
bash -n "$d/menu.sh"
printf '%s\n' 01d246bc1efb82571308432634b245f9869ce647357d6cdf8a55b8f3e1004127 > "$d/public.hex"
if test -e /etc/arcvpn/update-public.hex || test -L /etc/arcvpn/update-public.hex; then
  cmp "$d/public.hex" /etc/arcvpn/update-public.hex
else
  install -m 0644 -o root -g root "$d/public.hex" /etc/arcvpn/update-public.hex
fi
cp -p /usr/local/bin/arcvpn "$d/menu.previous"
mv -fT "$d/update.zip" /var/lib/arcvpn/update/staged.zip
systemctl start arcvpn-update.service
test "$(/opt/arcvpn/bin/arcvpn-core version)" = 'arcvpn 0.7.0-stage5-a5'
runuser -u arcvpn -- /opt/arcvpn/bin/arcvpn-core frontend-check
next=$(mktemp /usr/local/bin/.arcvpn-a5.XXXXXXXX)
install -m 0755 -o root -g root "$d/menu.sh" "$next"
mv -f "$next" /usr/local/bin/arcvpn
/opt/arcvpn/bin/arcvpn-core version
systemctl is-active arcvpn-panel.service x-ui.service
printf 'Previous menu: %s/menu.previous\n' "$d"
)
```

При ошибке не запускать installer и не повторять блок вслепую; прислать безопасный итог команды. Ожидается `arcvpn 0.7.0-stage5-a5`, `embedded React frontend: OK`, две строки `active`. После этого обновить страницу панели без кэша, войти старым аккаунтом, открыть «Подключения → Добавить подключение», затем пользователь → «Подключить». Если нужен свой логин/пароль — «Безопасность → Логин и пароль».

## Доказательства и границы

Сборка Linux/amd64 Ubuntu22.04, Go1.26.6/Node22.21.1: pinned npm ci, TypeScript/Vite, `go test ./...`, `go vet ./...`, embedded frontend/version/lifecycle и packaging PASS. Installer подпись проверена штатным offline verifier, update подпись и соответствие версии/hash core — независимо Node crypto. Контрольные суммы опубликованных assets закреплены выше. BUILDINFO `source_dirty=true`: в workspace был untracked `.serena/`, не включённый в source manifest/archive. Точный релизный snapshot приложен, не подменяется поздней документацией.

Команда обновления проверена синтаксически; реальное применение/rollback на VPS и клиентский VPN-трафик пока NOT RUN. [Предыдущие целевые проверки](../testing/local-results/stage5-2026-11-07-first-connection.md). При успешном обновлении active panel подтверждает запуск, но не подключение из Happ/Streisand.
