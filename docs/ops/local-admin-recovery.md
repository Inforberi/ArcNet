# Управление панелью через терминал сервера

Контракт исходников 2026-11-05. Команды доступны после установки обновлённого `/usr/local/bin/arcvpn` из `deploy/arcvpn-menu.sh` и соответствующего core. Уже установленный старый wrapper сам этим изменением не обновляется. Изменения не развёрнуты на реальном VPS.

Дополнение A1 2026-11-06: общие [start/stop/restart/status и журналы](local-service-control.md) реализованы в исходниках. Recovery/settings по-прежнему выполняются service user; root lifecycle вызывает только фиксированные системные действия и не открывает SQLite от root. Для нового lifecycle нужны также согласованные units/tmpfiles/cert hook, а не одна замена wrapper.

```bash
sudo arcvpn
sudo arcvpn settings
sudo arcvpn settings --json
sudo arcvpn status
sudo arcvpn password reset
sudo arcvpn sessions revoke
sudo arcvpn logs panel
sudo arcvpn logs xray
sudo arcvpn restart panel
sudo arcvpn restart xray
sudo arcvpn doctor
```

`settings` показывает логин, состояние 2FA, число действующих административных сессий, бюджет файлов журнала и основной VPN address/port. Wrapper получает PID панели через systemd и передаёт его в core; `/proc/PID/cmdline` преобразуется только в allowlist listen/public HTTPS origin/TLS configured. Другие аргументы и пути ключей не печатаются. Остановленная/недоступная панель даёт неизвестные runtime-параметры, без выдуманного URL. Метка «TLS задан» не является проверкой цепочки или доступности.

Пароль хранится как хеш, прежнее значение посмотреть невозможно. `password reset` генерирует случайный пароль и печатает его один раз в локальный терминал. Все административные сессии отзываются, активная 2FA сохраняется; незавершённая настройка второго фактора отменяется. Пользователи VPN, их ключи, подписки и работающие подключения не изменяются. Не перенаправляйте вывод с новым паролем в публичные журналы.

Для собственного пароля используйте интерактивное чтение без echo и stdin, а не аргумент команды или буквальный пароль в shell history:

```bash
IFS= read -r -s -p 'Новый пароль: ' arcvpn_new_password
printf '\n'
printf '%s\n' "$arcvpn_new_password" | sudo arcvpn password reset --password-stdin
unset arcvpn_new_password
```

От 8 Unicode code points, до 256 байт UTF-8 в одной строке (пробелы сохраняются); переданное значение core не выводит. Для автоматизации stdin должен завершаться EOF после строки. Встроенный интерактивный пункт меню генерирует пароль сам.

При утрате 2FA выполните отдельный явный сброс:

```bash
sudo arcvpn password reset --reset-2fa
```

Это также меняет пароль и отзывает сессии. После входа включите 2FA заново. В меню полный сброс отделён от обычного и требует ввода `RESET`. `sessions revoke` только завершает административные сессии, не меняя пароль и 2FA.

Граница прав: root-wrapper выполняет core через `runuser -u arcvpn`, с фиксированным `ARCVPN_DATA_DIR=/var/lib/arcvpn`. Прямой `admin-reset/settings/admin-revoke-sessions` от root/чужого пользователя отклоняется **до** открытия/создания БД. Требуется существующий обычный файл SQLite без group/world write. Recovery не создаёт новую установку вместо отсутствующей БД. Изменение хеша/MFA/сессий и запись аудита атомарны; ошибка откатывает всё. Аудит не содержит пароля, seed или токенов.

Core-команды для подготовленного окружения:

```bash
sudo -u arcvpn env ARCVPN_DATA_DIR=/var/lib/arcvpn /opt/arcvpn/bin/arcvpn-core settings
sudo -u arcvpn env ARCVPN_DATA_DIR=/var/lib/arcvpn /opt/arcvpn/bin/arcvpn-core admin-reset
```

`admin-reset` теперь сохраняет активную MFA по умолчанию; прежнее совместное удаление требует `--reset-2fa`. Системные restart/log commands ограничены панелью и Xray, произвольный unit не принимается. Журнал читается из `--namespace=arcvpn`.

Проверки: [локальные результаты](../testing/local-results/stage5-2026-11-05-dashboard-recovery.md). Настоящие systemd/SSH/ACME и установка/обновление wrapper на VPS ещё не приняты.
