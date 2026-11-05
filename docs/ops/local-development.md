# ArcVPN: локальный запуск и проверка

Эта инструкция запускает **разработческий** backend/React и SQLite на собственной машине, не изменяя systemd, UFW или сетевые правила VPS. Это не production-установка.

## Требования

Linux (или macOS с соответствующими библиотеками), Go 1.23+, gcc/clang, SQLite runtime/development library (`libsqlite3`), `libqrencode` и её headers/library, Node.js 22+ и npm. Для реальной сборки React нужен доступ к npm. Актуальные `npm`-версии сверять с `frontend/package.json`; `frontend/package-lock.json` в исходной версии пока отсутствует и должен быть создан в сетевой среде и зафиксирован Codex. Нельзя объявить сборку проверенной, пока это не сделано.

## Запуск (две службы, одна команда)

```bash
bash scripts/dev-local.sh
```

Скрипт создаёт `.local/data/panel.sqlite`, при первом запуске печатает пароль администратора, компилирует Go API на `http://127.0.0.1:8181`, поднимает Vite на `http://127.0.0.1:5173` с прокси `/api` и `/sub`. Останавливает Go backend при выходе. React берётся из `frontend/src`; Node/npm нужен только для разработки.

В тестовом HTTP режиме cookie не имеет признака `Secure`, поэтому **никогда не публикуйте 8181/5173 в интернет**. Параметр `--legacy-ui` используется только для обслуживания Go API на loopback: браузер открывает настоящий React через Vite.

## Проверки

```bash
bash scripts/verify-stage3-local.sh
CGO_ENABLED=1 go test -race ./internal/mfa ./internal/store ./internal/httpapi ./internal/logging ./internal/backupfile
```

Для полноценной React-сборки в сетевой среде (после генерации и фиксации lockfile):

```bash
cd frontend
npm install --package-lock-only --ignore-scripts --no-audit --no-fund
npm ci
npm run typecheck
npm run build
```

Для release-архива **не достаточно** сборки Go. `scripts/build-release.sh` должен встроить реальный Vite output и собрать закреплённые бинарники VPN. `install.sh` требует `dist/SHA256SUMS`, настоящий React и все движки, поэтому запуск `install.sh` из source-кандидата намеренно невозможен.

## Локальные зашифрованные копии (без systemd)

```bash
umask 077
mkdir -p .local/private .local/backups
printf '%s\n' 'очень-длинный-пароль-для-архива' > .local/private/backup-password
chmod 0600 .local/private/backup-password
ARCVPN_DATA_DIR="$PWD/.local/data" go run ./cmd/arcvpn backup-create --password-file "$PWD/.local/private/backup-password" --directory "$PWD/.local/backups" --keep 7
```

Указанный пароль — только пример для изолированной локальной разработки, не использовать для своих реальных архивов. На VPS генерировать отдельный случайный пароль. **Копию нужно уносить с VPS**: локальная копия не спасёт при потере хоста.

Полное восстановление и проверка доступности протоколов после смены IP относятся к внешней приёмке Codex.

## Журналы в production (только после приёмки на VM)

Установщик создаёт собственный `/etc/systemd/journald@arcvpn.conf.d/90-arcvpn-budget.conf` и добавляет `LogNamespace=arcvpn` для ArcVPN-служб. По умолчанию журнал namespace ограничен 200 МиБ, а ротация Go-панели — отдельные 500 МиБ. Из интерфейса лимиты меняются независимо. Команды поиска при установке с namespace:

```bash
sudo journalctl --namespace=arcvpn -u arcvpn-panel.service -n 100 --no-pager
sudo journalctl --namespace=arcvpn --disk-usage
```

Обычные `journalctl -u arcvpn-panel` могут не показывать сообщения после включения пространства имён, поэтому всегда указывать `--namespace=arcvpn`. Администратор VPS должен иметь доступ к systemd и root-owned конфигурации. На обычной машине без systemd endpoints настройки journal возвращают `503`, что ожидаемо. Обновление лимита через UI меняет **только** созданное ArcVPN пространство имён и может удалить старые архивные записи; не использовать без резервной копии и внешнего теста. Поведение уточняется по документации `journald@.conf` и `journalctl --namespace`.
