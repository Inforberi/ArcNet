# A3: bootstrap и публикация prerelease — 2026-11-06

**BOOTSTRAP_LOCAL_PASS / PUBLIC_DOWNLOAD_PASS / VPS_NOT_RUN.** Core A2 не пересобирался; предыдущие Go/frontend/release результаты относятся к A2 snapshot. Новых протоколов/API/БД/runtime в A3 нет.

## Выполнено

- `bash -n deploy/bootstrap-install.sh` и syntax установочной команды — exit0.
- `docker run --rm --network none -v "$PWD:/src:ro" -w /src arcvpn-local-test:20261105 python3 -u scripts/testing/bootstrap-fixture.py` — exit0. Настоящие OpenSSL Ed25519 и Python tar parser: корректный подписанный payload принят; подписанные traversal/symlink/hardlink/duplicate/setuid/unsafe checksum paths отклонены; подмена archive/signature отклонена. Offline source без verify-only не разрешает install; выхода из временного каталога нет. Fixture не исполняет install.sh/core.
- После отделения RELEASE_VERSION от системного VERSION выполнен только повтор затронутого fixture, exit0. Это предотвращает замену закреплённой версии при чтении `/etc/os-release`.
- Создан и опубликован GitHub prerelease [v0.7.0-stage5-a2](https://github.com/Inforberi/ArcNet/releases/tag/v0.7.0-stage5-a2): API подтвердил `draft:false`, `prerelease:true`; загружено12 allowlisted assets, включая exact A2 source snapshot и отдельный bootstrap. Private key не передавался API/в контейнер и не публиковался.
- Один настоящий публичный download smoke из read-only Linux-контейнера с tmpfs160MiB: bootstrap скачан по закреплённому HTTPS release URL, SHA256 проверен, `bash /tmp/install.sh --verify-only` — exit0, `PASS: signature, pinned version, archive and payload integrity; safe extraction`. Использован настоящий4.1MiB опубликованный пакет, не fixture; payload не исполнялся.
- `git diff --check` — exit0. Исходный manifest обновлён вместе с A3; опубликованный A2 manifest не менялся.

Bootstrap SHA256: `db5914d3a3ee6e4e6af0e3832f509eb0459859c565b82dbb8c85a96cf500eb87`. Подпись проверяется до использования metadata, архив ограничен128MiB, expanded tar96MiB; разрешены только14 конкретных regular files. Нет `extractall`, link/special/permissive-mode extraction, key/URL overrides или исполнения скачанного verifier для самопроверки.

## Не проверено / границы

Реальная привилегированная установка, systemd, apt/runtime downloads, IP ACME/renewal, VPN handshake/egress, reboot, восстановление backup и профиль512МиБ/1vCPU **NOT RUN**. Это пользовательская [короткая приёмка](../../ops/vps-preview-acceptance.md). Bootstrap удаляет временные файлы; не реализован полный rollback изменений install.sh/ОС или in-place migration. Поздний домен не автоматизирован этим блоком. Каскады/Geo/failover остаются открытыми. Внешний stage4/5 ACCEPTED не выставлен.
