# ADR-011: локальный WireGuard — устройства и расход

Объём по прямому решению пользователя: только WireGuard. AmneziaWG исключён из дальнейшего плана этой задачи; старые данные/реализация не удаляются. Remote nodes и каскады вне блока.

## Credentials и адреса

Устройство получает отдельные X25519 private/public keys и IP. Ключи генерируются криптографически, не выводятся из UUID. Адрес выделяется в транзакции из существующей `10.76.0.0/16`, без новых маршрутов/firewall/подсетей. Legacy keys/IP сохраняются; новые legacy credentials получают свободный IP при конфликте с устройством. Адреса отозванных устройств пока не переиспользуются. Pool exhaustion — явный отказ, не дубликат. Валидация клиента допускает адрес внутри управляемого /16, а не только старую формулу ID+1; AmneziaWG не меняется.

Device mode исключает общий legacy peer. Generated server config содержит только включённых пользователей с действующим пакетом и неотозванные устройства. В UI переиспользован DevicesDialog: имя/лимит/отзыв, IP и отдельное скачивание `.conf`. Обобщённая VLESS/Hysteria subscription не подменяется WG-конфигом. Private key выдаётся только через явный secret-bearing device token export, с проверкой родителя/пакета/отзыва и no-store. Списочный API private keys/tokens не содержит.

## Источник трафика и привилегии

Контракт CLI проверен Context7 по [wireguard-tools](https://git.zx2c4.com/wireguard-tools/tree/src/show.c): фиксированные `wg show arcwg public-key` и `wg show arcwg transfer` (public key, server rx/client upload, server tx/client download). Никогда не используются `dump`, `private-key` или `preshared-keys`. Команды без shell, с timeout/output bounds; наружу передаются только public keys/counters/epoch/config digest.

В существующий root helper добавлен один allowlisted read-only запрос `stats wireguard`. Клиент проверяет root-owned socket и parent directory без group/other write. Существующий `tunnel up` после успешного wg-quick создаёт root-owned epoch receipt в `/run/arcvpn/wireguard-epoch.json`: boot ID, interface index, случайная nonce, время и SHA256 проверенного root config. Ошибка stamp вызывает ограниченный `wg-quick down`; успех без receipt не объявляется. `down` удаляет receipt. При миграции работающего интерфейса без receipt Manager запрашивает контролируемый restart. Это затрагивает только WireGuard.

Root stats сверяет текущий boot/interface, private root config digest, фактический server public key и точный набор peers; проверяет epoch/interface ещё раз после чтения. Backend сверяет root report с SHA собственного generated config. Чужая ревизия, отсутствующие peers, дубликаты, регрессия или ошибка helper — unknown. Receipt не является внешним VPN handshake proof. Kernel или host root остаются доверенной границей.

## Учёт, восстановление и ограничения

Новый stream `wireguard` суммирует per-peer counters на пользователя. Freshness WG независима от Xray/Hysteria: конечный общий пакет требует всех включённых источников. Replay не списывает повторно; неожиданный новый interface epoch сохраняет расход и ставит needs_review. Управляемый apply снимает доступный tail и фиксирует новую epoch по прежнему контракту incomplete. Неизмеренный хвост при restart возможен; точный byte overshoot не заявлен. Ошибка apply/helper остаётся pending, старый трафик может продолжаться до успешного применения.

Backup v9 переносит device keypairs/IP/checkpoints, читает v1–v8. Новые WG fields/capability нельзя пометить старой версией. Старый backup не ротирует уже известные локальные ключи устройств, не отменяет отзывы и не уменьшает известный расход. Коллизии device/legacy адресов и ключей отклоняются. Миграция полей выполняется автоматически при наличии WG profile, без начальной формы настройки.

Unit/API/fixture checks не заменяют root-helper rollback на изолированной VM, kernel WG handshake, systemd и реальный трафик на VPS. До этих проверок WireGuard runtime и enforcement не считаются ACCEPTED. Новые внешние сервисы, зависимости и настройки сети не добавлены.
