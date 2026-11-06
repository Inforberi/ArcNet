# Короткая приёмка ArcVPN на тестовом VPS

Тестовый [prerelease 0.7.0-stage5-a2](https://github.com/Inforberi/ArcNet/releases/tag/v0.7.0-stage5-a2). Debian12+/Ubuntu22.04+, x86_64, systemd, публичный IPv4; TCP/80 для ACME и TCP/8443 для панели доступны снаружи. До запуска сохраните snapshot и доступ к консоли. Проверку проводит владелец сервера; успешная локальная сборка не подтверждает VPS/VPN.

## Установка одной командой

Выполните **от root** (при необходимости сначала `sudo -i`):

```bash
bash -c 'set -euo pipefail; [ "$(id -u)" = 0 ] || { echo "Run as root" >&2; exit 1; }; umask 077; d=$(mktemp -d /var/tmp/arcvpn-launch.XXXXXXXX); trap '"'"'rm -rf -- "$d"'"'"' EXIT; command -v curl >/dev/null || { apt-get update -qq; apt-get install -y --no-install-recommends curl ca-certificates; }; curl --proto "=https" --proto-redir "=https" --tlsv1.2 -fsSL --connect-timeout 15 --max-time 60 --max-filesize 131072 https://github.com/Inforberi/ArcNet/releases/download/v0.7.0-stage5-a2/arcvpn-install-0.7.0-stage5-a2.sh | head -c 131073 > "$d/install.sh"; printf "%s  %s\n" db5914d3a3ee6e4e6af0e3832f509eb0459859c565b82dbb8c85a96cf500eb87 "$d/install.sh" | sha256sum -c -; bash "$d/install.sh"'
```

Команда закрепляет SHA256 bootstrap; bootstrap содержит доверенный Ed25519 public key, скачивает конкретную версию, проверяет подпись manifest/размер/SHA256, безопасно извлекает строго известные файлы и запускает install.sh. Не заменяйте hash значением из непроверенного скачанного файла. Go/Node/Docker на VPS не нужны. При отсутствии curl он устанавливается из репозитория ОС. Другие зависимости ставит установщик; Xray/Hysteria загружаются отдельно, это не offline bundle.

Существующая установка отклоняется. Это не in-place upgrade. При прерывании installer возможны частичные изменения ОС: сохраните диагностику, восстановите snapshot; автоматический rollback ОС не заявлен. Перенос старой панели — [чистая установка + portable restore](installer-release-and-migration.md).

Без установки: скачайте bootstrap, проверьте указанный SHA256 и выполните `bash arcvpn-install-0.7.0-stage5-a2.sh --verify-only`. Для уже скачанного набора допускается `--verify-only --from-dir /path/to/assets`; payload при этом не исполняется, зависимости не устанавливаются. В Linux нужны openssl, python3, timeout и curl для сетевой проверки.

## Приёмка

1. Сохраните напечатанные URL/логин/пароль вне публичных логов. Откройте HTTPS, включите 2FA. Проверьте UI с телефона, дашборд и `sudo arcvpn status`; active unit сам по себе не доказывает трафик.
2. Создайте первым нужный VLESS/REALITY TCP, XHTTP, Hysteria2 или WireGuard, затем пользователя/устройство. Подключитесь независимым клиентом, откройте сайт и проверьте внешний IP и DNS. Повторите для реально нужных протоколов; проверьте отзыв устройства и учёт трафика. AmneziaWG не включён. Каскады, Geo и автоматический failover этой сборкой не приняты.
3. Выполните по очереди:

```bash
sudo arcvpn status
sudo arcvpn stop
sudo arcvpn start
sudo arcvpn restart
sudo arcvpn settings
sudo arcvpn doctor
sudo arcvpn logs panel
```

После stop проверьте отсутствие VPN и доступность SSH; после start/restart — реальное переподключение. SSH, удалённые ноды и таймеры backup/renewal не останавливаются. Accounting review gate при разрыве учёта требует отдельного разбора, не обходите его повторными рестартами. Проверьте восстановление после reboot. [Контракт CLI](local-service-control.md).

4. TLS: по умолчанию установщик запрашивает короткоживущий IP-сертификат Let's Encrypt (профиль shortlived, около шести дней). Проверьте сертификат браузером, `sudo systemctl list-timers arcvpn-cert-renew.timer` и `sudo systemctl status arcvpn-cert-renew.service`; не публикуйте приватные ключи. Домен и перевод renewal на него не автоматизированы этим bootstrap — отдельная конфигурация/приёмка, не обязательный шаг первого запуска.
5. Backup: `sudo arcvpn` → пункт6 создаёт зашифрованную VPN-копию, пункт8 включает ежедневные копии. Скопируйте архив из `/var/backups/arcvpn` вне VPS и сохраните пароль отдельно. В UI «Резервные копии» загрузите архив, введите пароль, проверьте preview/новый адрес и явно подтвердите замену. Восстановление проверяйте на отдельной чистой установке или snapshot: VPN/пользователи заменяются; настройки ОС, админский доступ и TLS не восстанавливаются одним portable backup. Проверьте подключения и сохранение расходов/отзывов после restore.
6. Recovery: `sudo arcvpn password reset` меняет пароль и отзывает сессии, сохраняя 2FA; при утрате 2FA — явный `sudo arcvpn password reset --reset-2fa`. [Подробности](local-admin-recovery.md).

Запишите ОС, RAM/CPU, версию, результат установки/подключения/lifecycle/TLS/restore и ошибки без секретов. Профиль512МиБ/1vCPU пока не подтверждён серверным замером. До этой приёмки релиз остаётся preview.

## Происхождение пакета

Core уже собран и подписан в A2; A3 его не пересобирает. BUILDINFO содержит `source_dirty:true`, commit `f3946a8` и точный hash исходного snapshot. Asset `arcvpn-0.7.0-stage5-a2-source.tar.gz` — именно исходники сборки; tag8a5cfcb относится к A2 handoff, bootstrap/эта инструкция добавлены отдельно. Не отождествляйте tag с чистой сборкой бинарника. Public installer signer fingerprint: `3f0f85ed250a916bfdeefd78d8eb15f84b90e23f67939ae3b3c25ac62f933c24`. Подпись installer не включает автоматическое доверие будущим updater-пакетам; update key в A2 отсутствует.
