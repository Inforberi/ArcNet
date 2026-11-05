# ADR-010: локальный учёт и устройства Hysteria2

2026-11-05. Продолжение ADR-008/009; WG/AWG, remote nodes и каскадный runtime вне объёма.

## Источник и доверие

Проверенная версия — официальный Hysteria **app/v2.13.0**. Контракт [trafficStats](https://hysteria.network/docs/advanced/Traffic-Stats-API/) подтверждён Context7, официальными исходниками и настоящим локальным QUIC/TLS тестом. `GET /traffic?clear=false`, Authorization — машинный случайный secret; tx — upload клиента, rx — download. Счётчики upstream не сбрасываются панелью. Stats API не публикуется в browser/agent HTTP.

В этой версии listener только TCP: [реализация](https://github.com/HyNetworks/hysteria/blob/app/v2.13.0/extras/correctnet/correctnet.go). Автоматический private endpoint — `127.0.0.1:19823`, без начальной настройки и без UFW изменений. Перед передачей секрета проверяется socket inode в `/proc/<MainPID>/fd`, его loopback LISTEN entry и UID; проверка повторяется после TCP connect до HTTP Authorization. Это исключает принятие чужого localhost HTTP-сервиса за Hysteria. MainPID, InvocationID и время старта берутся фиксированной read-only командой `systemctl show arcvpn-hysteria.service`; epoch сверяется до/после запроса. Требуются видимый Linux procfs и systemd с `--timestamp=us+utc` (248+ по [документации](https://www.freedesktop.org/software/systemd/man/latest/systemctl.html#--timestamp=)). Неподдерживаемая среда даёт unknown, не фиктивный расход.

Config и машинный key читаются только из private runtime directory; файл не может быть symlink или доступным группе/прочим. HTTP без proxy/redirect/keepalive, с timeout, bounded body и фиксированным адресом. Машинный key не попадает в portable backup. Канонический server JSON хранится в прежнем `hysteria.yaml` (JSON — допустимый YAML document); реальные Hysteria parser/start подтверждены тестом. Auth maps сверяются с известными credentials; изменение config/process во время чтения отклоняется.

## Учёт и исполнение

Новый stream `hysteria2` использует прежние atomic checkpoints/debits/общий пакет пользователя. Replay не списывает повторно, регрессия отклоняется. Xray и Hysteria имеют независимые freshness settings; исправный Xray не делает отсутствующий Hysteria свежим. Требуется свежесть всех включённых поддержанных движков. Ошибка одного источника не мешает сохранять уже полученные байты другого. WG/AWG и зарегистрированные ноды по-прежнему закрывают coverage для конечных пакетов/device mode.

Перед apply снимается доступный Hysteria tail, после успеха с ограниченным ожиданием фиксируется новая epoch. Пропуск при restart отмечается incomplete; неожиданный restart — needs_review с закрытием конечного пакета до явного нового периода. Если post-apply наблюдение не получено, fake source ACK не создаётся. Запуск/остановка/перезапуск происходят через прежний allowlisted helper; новых privileged RPC нет. Неудачный apply остаётся pending и повторяется общим worker.

Отзыв/expiry/quota применяются через generated allowlist и restart движка. Это может переподключать остальные клиенты того же Hysteria процесса; встроенный reconnect клиента не отменяет проверку нового allowlist. Копирование credential на другое физическое устройство не исключается. Общий byte overshoot и время отзыва на VPS не заявлены. Неснятый хвост между последним counter read и restart возможен; subscription header не выдаёт неполный расход как полный.

## Устройства, UI и backup

Legacy username `u<userID>` и его secret сохраняются до перехода на device mode. Для устройства используется `d<deviceID>` и отдельный UUID secret; общий legacy credential затем отсутствует в конфигурации. Имена/даты/лимиты/отзывы остаются в существующем DevicesDialog, индивидуальная подписка содержит TCP/XHTTP и Hysteria, если они включены. Учёт суммирует также известные retired aliases до смены epoch. Отдельный «online устройства» не выдумывается.

Backup **v8** переносит Hysteria checkpoints и прежние пакеты/устройства; v1–v7 читаются. Старые версии payload не принимают новый stream под видом совместимого формата. Местные отзывы/high-water расхода, CAS и точная компенсация неудачного импорта сохраняются. Restore очищает liveness обоих источников.

Source/loopback/UI проверки не являются внешней VPS/systemd/ACME приёмкой. Бинарник для локального теста SHA256 `a68a61a84452ca250ce0368202521965ca9cc9d801a404f1dc9008ac6cf677a7` из официального release asset; новых зависимостей проекта нет.
