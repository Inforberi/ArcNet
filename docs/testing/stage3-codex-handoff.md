# Stage 3 — внешний handoff в Codex

**Source:** 0.5.1-stage3-code-handoff; **статус:** `LOCAL_PASS / EXTERNAL_PENDING`. Работать только с копией полного проекта и с отдельным VPS. Не менять действующую 3x-ui. Сделать snapshot и гарантировать rescue console до тестирования firewall.

## 1. На сетевой сборочной машине

```bash
cd frontend
npm install --package-lock-only --ignore-scripts --no-audit --no-fund
npm ci
npm run typecheck
npm run build
cd ..
bash scripts/verify-stage3-local.sh
./scripts/build-release.sh
./dist/arcvpn-linux-amd64 frontend-check
python3 tests/react_e2e.py
```

Закоммитить `frontend/package-lock.json`, затем rerun с чистым `npm ci`. Go 1.25+ для AWG userspace, системные `libsqlite3-dev`, `libqrencode`, C toolchain, Git. Убедиться, что компоненты AmneziaWG собраны из зафиксированных upstream тэгов, а SHA256SUMS соответствует реально доставляемым файлам. Проверить лицензии при распространении.

## 2. На одном отдельном VPS

- Установить из готового релиза, проверить `frontend-check`, `journalctl`, systemd unit `arcvpn-update.service`: `RuntimeDirectory=arcvpn-update`, запись lock при `ProtectSystem=strict`, отсутствие произвольных команд от Go-панели.
- Проверить настоящий Certbot `--ip-address`, порт 80, короткий сертификат, продление, права ключей, HTTPS **без** `-k`; панель недоступна по незащищённому внешнему HTTP.
- TOTP: настройка, подтверждение, отклонение повтора, инвалидирование сессий, CLI `admin-reset` (сохраняет активную MFA); удаление MFA только с явным `--reset-2fa`, оба пути и отзыв сессий проверить на VPS.
- UFW: перед активированием зафиксировать SSH custom port, проверить rescue console и snap. Из UI открыть/закрыть только VPN-порт, проверить чужие правила, отсутствие глобального enable/reset, rollback и отсутствие lockout.
- Журналы: `journalctl --namespace=arcvpn --disk-usage`, смена budget 50↔200, доступ к панели, отсутствие изменений в SSH/global journald, рестарт службы, удаление старых файлов журнала панели.
- Backup: зашифрованный snapshot, SHA/файловые права, таймер, миграция на второй чистый VPS, смена IPv4 и перепривязка ссылок/агентов, тест отказа Xray при restore и отката.
- Signed update: сформировать offline ключ на доверенной машине; скопировать только public key на тестовый VPS; подписать валидный пакет, отвергнуть неподписанный/подменённый; применить update core+движков, после failure восстановить старый бинарник и подтвердить реальный VPN-трафик. Отдельно проверить отказ при отсутствии RuntimeDirectory/запрещённом доступе.
- Реально подключить клиентов по VLESS Reality, XHTTP, Hysteria2, WireGuard и AmneziaWG 3.1. Проверить DNS, NAT, UDP, переподключение и доступность после reboot. Статус службы не заменяет этот тест.

## 3. Браузерная приёмка

На 320/390/768/1024/1440 px проверить React, темы через cookie, администратора/2FA, VPN-профили, QR, подписки, копирование, пользователей, UFW, журнал, backup и signed-update. Все кнопки должны выполнять заявленные операции; отсутствие горизонтального переполнения, ошибок JS и dead-end диалогов. Скриншоты хранить **без секретов и токенов**.

## 4. Отчёт

Создать `docs/testing/external-results/stage3-<date>.md` с командами, ОС, версиями, скриншотами без секретов, результатами `PASS`/`FAIL`/`BLOCKED`/`NOT RUN` по каждому пункту. При ошибке исправить исходники, повторить весь затронутый сценарий. Только после этого допустим `ACCEPTED`.
