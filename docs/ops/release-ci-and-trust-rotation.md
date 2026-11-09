# Выпуск из GitHub и явная смена ключа

9 ноября 2026 владелец разрешил новый signer и его передачу в GitHub Environment Secret. Это заменяет прежний сценарий с ключом на другом компьютере. Подписи не отключаются; доверие существующих панелей автоматически не меняется.

## Настройка

[Publish signed ArcNet update](https://github.com/Inforberi/ArcNet/actions/workflows/release.yml): ручной запуск только из main, concurrency без отмены уже идущего выпуска. Environment `arcnet-release` разрешает только main, Secret `ARCVPN_SIGNING_KEY_HEX` доступен только publish job. Build без ключа/write token и publish работают на разных runners; Actions закреплены по официальным SHA. Проверяются exact source commit/clean flag/SOURCE_MANIFEST, Linux ELF/core SHA и frontend/version receipt. Каталог публикуется последним, PR/fork artifacts не используются.

Новый SHA256 сырых32 public bytes: `fd1a8f9bdcfb36b38eeead9fa13d990c8e2d004edb43758d405b0297d0961021`.
SHA256 файла `deploy/update-public.hex`: `16be6177a7cc2a739d1ff4dcb1e093660f4e9158a4398a955cda517e0b874d52`.
Прежний public key/подписанный каталог сохранены в `deploy/trust/legacy-2026-11-09/`. Старые ZIP не переподписываются и не авторизуются новым ключом. Новые каталоги сначала пусты, затем пополняются реально выпущенными пакетами.

Приватный ключ не включается в Git/Docker/artifacts/VPS. Локальный backup: `$HOME/.local/share/arcvpn-release-signing-2026-11-09/installer-private.hex`,0600/каталог0700. Доверяем GitHub и владельцам main/release workflow. Secret нельзя скачать обратно: нужна резервная копия. При компрометации — новый явный переход доверия.

## Выпуск

Actions → Publish signed ArcNet update → Run workflow → main:

1. `release_type=bridge`: A10/`update-a10.zip`/v1 catalog для A7–A9 после смены ключа.
2. Установить A10 на тестовую панель, проверить фактическую версию/сохранённый доступ.
3. `release_type=stable`: VERSION (сейчас0.7.1)/`update-0.7.1.zip`/v2 catalog.

Следующие версии: VERSION/notes/SOURCE_MANIFEST → commit/push → stable. При авторизации: `gh workflow run release.yml --repo Inforberi/ArcNet --ref main -f release_type=stable`. Применение на VPS — по подтверждению пользователя. Core/UI не обновляет installer/bootstrap/units/hooks.

## Существующая панель: один раз сменить доверие

Без этого старые панели правильно отклонят новый каталог. Приватный ключ серверу не нужен. Сначала backup/snapshot; не менять trust во время обновления. Подготовленный ZIP отменить в панели, не удалять вслепую.

На сервере выполнить один явный административный переход. Публичный файл скачивается в root-owned временный каталог и проверяется там до замены; прежний ключ сохраняется. Команда откажется при занятом lifecycle lock, подготовленном ZIP, активном update или существующей резервной копии:

```bash
sudo bash <<'BASH'
set -Eeuo pipefail
umask 077
test -d /run/arcvpn-lifecycle
test ! -L /run/arcvpn-lifecycle/lock
exec 9>/run/arcvpn-lifecycle/lock
flock -n 9
case "$(systemctl show --value -p ActiveState arcvpn-update.service)" in
  inactive|failed) ;; *) echo 'Update service is busy or unknown'; exit 1;;
esac
test ! -e /var/lib/arcvpn/update/staged.zip
test -f /etc/arcvpn/update-public.hex
test ! -L /etc/arcvpn/update-public.hex
test ! -e /etc/arcvpn/update-public.before-20261109.hex
work=$(mktemp -d /etc/arcvpn/.trust-rotate.XXXXXXXX)
trap 'rm -rf -- "$work"' EXIT
curl --proto '=https' --tlsv1.2 -fLsS --connect-timeout 10 --max-time 30 --max-filesize 65 \
  https://raw.githubusercontent.com/Inforberi/ArcNet/main/deploy/update-public.hex \
  -o "$work/key.hex"
printf '%s  %s\n' '16be6177a7cc2a739d1ff4dcb1e093660f4e9158a4398a955cda517e0b874d52' "$work/key.hex" | sha256sum -c -
cp -p -- /etc/arcvpn/update-public.hex /etc/arcvpn/update-public.before-20261109.hex
install -o root -g root -m 0644 -- "$work/key.hex" "$work/installed.hex"
mv -T -- "$work/installed.hex" /etc/arcvpn/update-public.hex
sha256sum /etc/arcvpn/update-public.hex
BASH
```

Итоговый SHA — `16be6177a7cc2a739d1ff4dcb1e093660f4e9158a4398a955cda517e0b874d52`. При отказе не отключать проверки: разберитесь с подготовленным пакетом/службой/путём установки. Команда использует стандартные пути; для нестандартного data dir их должен сверить администратор. Это явная смена trust, не повторный installer. Здесь серверные команды не выполнялись; VPS-проверка остаётся владельцу.

После публикации A10: обновления (на A8/A9 «Версии») → проверить → скачать A10 → подтвердить. После перезапуска «Настройки → Обновления», затем0.7.1 после выпуска. A5/A6 требуют прежнего первоначального перехода.

Сохранённый public key позволяет администратору вернуть прежнее доверие, если новый пакет ещё не применён. Это не откат бинарника/данных; старый каталог после ротации доступен только в архиве. Исторический installer/bootstrap использует старый anchor: новый installer — отдельный будущий выпуск, не подмена assets.

## Ошибка выпуска

Не перезапускать вслепую и не перезаписывать теги/ZIP. Artifact `signed-publication-receipts` содержит подписанные bytes для восстановления; [порядок](release-update.md#если-выпуск-прервался). Выпуск не доказывает systemd/VPS apply/rollback.

Основания: [GitHub Secrets API](https://docs.github.com/en/rest/actions/secrets), [workflow dispatch](https://docs.github.com/en/actions/how-tos/write-workflows/choose-when-workflows-run/trigger-a-workflow).

## Фактические проверки этого блока

Environment/Secret настроены через GitHub API после отдельного явного согласия владельца; branch policy main подтверждена чтением. Native `CGO_ENABLED=0 go build` и `go test ./cmd/arcvpn-release` PASS. Обычные Linux CGO `go test ./internal/lifecycle ./internal/updater ./cmd/arcvpn-release` PASS; после добавления cross-signer regression полный updater повторён PASS. `bash -n` и actionlint1.7.12 PASS. CLI `release-update.sh --from-build` с заведомо другим source_commit отказал до подписания/публикации, exit1. Run первого A10: https://github.com/Inforberi/ArcNet/actions/runs/42601831575 — **SUCCESS**, build/publish прошли. Опубликован A10, ZIP4792793 bytes, SHA256 `270340ee80f1db268ddb1afc7ae7e29f88ede89072226629cdfa167309edc7d1`; Release не draft, prerelease. Автоматический catalog commit992673b получен локально fast-forward. Full release pipeline Go tests/vet/React/frontend-check/version и signed-package verification PASS; asset скачан с GitHub и сравнен до публикации. Исходный snapshot b53a3b8. 0.7.1 опубликован: [run42615939048](https://github.com/Inforberi/ArcNet/actions/runs/42615939048) SUCCESS, [Release](https://github.com/Inforberi/ArcNet/releases/tag/v0.7.1). Source99110b2, catalog25d9e7b, ZIP4828457 bytes; SHA256 `812c7c08652022ddba91c4f2eae51afbef0b3df467bd9f6c03bc9b4c4c712c2c`. Реальная смена root key/systemd/VPS apply NOT RUN.
