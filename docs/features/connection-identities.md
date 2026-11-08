# Несколько локальных VLESS-подключений

Локальные исходники 8 ноября 2026. В этом блоке несколько VLESS/REALITY TCP и XHTTP, не несколько Hysteria/WireGuard. Последние требуют отдельной поддержки служб/root helpers и внешней сетевой приёмки. Новые тесты не добавлялись по запросу владельца.

## Идентичность и совместимость

Protocol.kind — тип протокола, Protocol.id — ID дополнительного профиля вида vless-reality:UUIDv4 либо vless-xhttp:UUIDv4. ConnectionID() возвращает id или прежний kind для оригинальных профилей. Исходный TCP остаётся в config; существующие XHTTP/Hysteria/WG строки не переписываются. В историческом столбце protocols.kind теперь хранится ключ профиля: алиас либо ID. Он же используется FK protocol_credentials.kind и ключами settings. Деструктивной SQL-миграции нет; новые API/DTO различают тип и ID. Старые primary aliases/ключи/UUID/подписки сохраняются.

Каждое дополнительное VLESS-подключение получает отдельную серверную ключевую пару/Short ID и параметры. Существующий UUID пользователя/устройства используется в его профилях; это не новые физические устройства и не независимые пользовательские пароли на каждую строку. Назначение и отзыв по ID исполняются списком разрешённых клиентов конкретного inbound, не только фильтрацией подписки.

Поддержаны до32 записей в protocols плюс исторический TCP config; это ограничение объёма операций/данных, не benchmark максимального числа подключений VPS. Hysteria/WG остаются единичными. Дополнительный VLESS использует тот же Xray process/helper/stats socket, без новых root-служб.

## API и интерфейс

POST /api/protocols создаёт дополнительный TCP VLESS или следующий XHTTP; первый исторический TCP по-прежнему POST /api/vpn. GET protocols возвращает base kind и optional id. PUT/DELETE /api/connections/{key}, rotation/health/export используют ID либо старый алиас; revision привязан и к ID. Настройки, fingerprint, path, Host, flow и публикация изолированы по ключу. Новые TCP-порты проверяются против primary, прочих listeners, HTTPS/ACME/private statistics. UFW автоматически ещё не согласовывается.

UserPreferences.allowed_kinds/direct_kind сохраняют старые имена JSON, но принимают и ID. [] запрещает все, null наследует все, включая новые профили. Синтаксис ID и его существование проверяются сервером; remote assignment guards сохранены. Снятие назначения удаляет клиента из соответствующего inbound после apply; shared UUID не обходит проверку списка клиентов.

UI позволяет снова выбрать TCP/XHTTP; state/React keys/изменение/удаление/rotation привязаны к ID, а тип определяет поля формы. В выборе пользователя и bulk видны тип и порт. Новые URI имеют различимые названия с портом. Старые ссылки оригинальных подключений не переименовываются.

Удаление только disabled. Для дополнительного профиля транзакция также удаляет его из явных назначений и сбрасывает direct preference в подписку, сохраняя пользователя/UUID/токен/пакет/другие назначения. Создание, изменение состояния и удаление обёрнуты publication epoch, чтобы HTTP export не выдавал промежуточные данные. Никакой автоматической ротации при редактировании.

## Трафик и перенос

Все VLESS inbounds используют существующие пользовательские email counters одного Xray, поэтому общий пакет не умножается на число подключений. Новые SQL coverage/device/schedule guards распознают оба типа дополнительных VLESS. Отдельный график каждого inbound не добавлен; online/физические устройства/IP enforcement не появляются из наличия ID. Health опрашивает каждую systemd-службу один раз на запрос, не по разу на каждый inbound.

Backup21 переносит ID, settings, credentials и назначения. Старые1–20 читаются с прежними aliases; новые ID под старой версией, несовпадение ID/type, повтор ID, конфликт порта и ссылки на отсутствующий дополнительный ID отклоняются. Старый reader не понимает21; откат только бинарника не является миграцией состояния назад.

## Проверено и открыто

Существующие Go tests семи затронутых packages — PASS; после исправлений export/delete/publication повторены затронутые app/httpapi/store cases — PASS. Frontend30 существующих tests и TypeScript/Vite — PASS. Точные команды/границы: current-task и change-log. Новых тестов не писали. Дополнительные реальные inbounds, клиентский handshake/отзыв/расход/reboot, изолированная VM rollback и browser/mobile acceptance не проверены. Это source implementation, не production acceptance. UI fixture синтетический, его mock Xray не реализует реальную генерацию ключей или VPN.

Команды на текущей рабочей копии (не immutable release snapshot):

```sh
go test ./internal/store ./internal/protocols ./internal/xray ./internal/engines ./internal/app ./internal/httpapi ./internal/backup -count=1 -timeout 90s
go test ./internal/app ./internal/httpapi ./internal/store -run 'ConnectionEdit|KeyRotation|Protocol|StableExport|UserEditor|UserBatch' -count=1 -timeout 60s
# frontend
npm test
npm run build
```

Go запускался в прежнем Linux image arcnet-remote-tests:20261106. Все перечисленные запуски PASS; существующий opt-in UI fixture пересобран, GET localhost:18394/vpn вернул200. Функциональную приёмку новых дополнительных профилей это не заменяет.
