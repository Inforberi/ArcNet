# A4: продолжение после сбоя ACME

Дата: 2026-11-07. Базовый release: `v0.7.0-stage5-a4`, core/source `9847050`.

## Наблюдения VPS, предоставленные оператором через Termius

- Первичная установка: exit 1, панель inactive, существующая 3x-ui active.
- Certbot 5.9.0: сначала ACME ReadTimeout 45s, затем TLS ConnectionResetError до HTTP-ответа.
- ACME directory: curl IPv4 HTTP 200; curl IPv6 timeout после 5s, HTTP 000.
- Тот же установленный certbot с `urllib3.util.connection.HAS_IPV6=False` только для процесса получил IP-сертификат; заявленный срок до 2026-11-13. Системный IPv6 не отключался.

## Подготовленное продолжение

`deploy/recover-a4-certificate.sh`: одноразовый recovery до появления units/данных, fixed A4 archive SHA256, сравнение core, проверка сертификата, исходное продолжение A4 после Certbot. Отдельный systemd drop-in сохраняет IPv4-only поведение только renew процесса; TLS validation сохраняется. Админский пароль не выводится в общую диагностику.

Фактически выполнены:

```bash
bash -n deploy/recover-a4-certificate.sh
docker run --rm --network none -v "$PWD:/work:ro" arcnet-remote-tests:20261106 bash /work/scripts/testing/recover-a4-fixture.sh
```

PASS: отказы при отсутствии состояния/runtime, наличии БД или units до записи drop-in; archive SHA256, точная граница продолжения A4, Bash syntax продолжения. Контейнер disposable, сеть выключена. Это не systemd/ACME/VPS rollback PASS.

VPS: recovery v2 выполнился, но преждевременно сообщил успех. Фактическая ошибка 203/EXEC: /opt/arcvpn root:root 0700 не пропускал service user; admin-init также не исполнился. После chmod 0755 только этого каталога и отдельного admin-init панель active/running, ExecMainStatus=0. Control socket, renew timer и сохранённая 3x-ui active. Внешний curl: HTTP 200, TLS verify 0, без TLS bypass.

Исправлены исходники install.sh: явные права OPT и bin, IPv4-only wrapper для certbot, renew только IP-сертификата ArcVPN. Recovery исправляет права и запускает source вне if, чтобы не подавлять errexit. Добавлены PASS-регрессии: unprivileged traversal, false в continuation не выполняет следующую команду, Certbot wrapper выбирает IPv4 до dispatch и отказывает при неизвестном API. Архив A4 не заменён; эти исходные исправления требуют следующего релиза.

NOT RUN: вход в UI, фактическое renewal через systemd, реальные VPN/расход/отзыв/reboot/restore/update. Основной runtime backlog остаётся открытым.
