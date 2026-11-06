# ADR-014: подпись полного installer и baseline Ubuntu22.04

2026-11-06, A2. [Release/migration guide](../ops/installer-release-and-migration.md), [фактические результаты](../testing/local-results/stage5-2026-11-06-installer-bundle.md).

Installer включает root shell/helpers, поэтому core-only update signature недостаточна. Введён `arcvpn-installer-v1` manifest с Ed25519 над `SignatureDomain || raw JSON`, где domain — `ArcVPN installer manifest v1\n`. Метаданные ограничивают version, filename, linux/amd64, размер и hash архива, source manifest hash. Подпись не переносится в протокол updater и не заменяет pin публичного ключа.

`internal/releasebundle` и pure-Go `cmd/arcvpn-release` предназначены для offline build host. Private key вне Git/Docker/VPS; bounded/no-follow files, exclusive output, проверка полного archive hash без исполнения. Публичный ключ рядом с тем же недоверенным archive не даёт независимого доверия. Первичная публикация/пин ключа и bootstrap относятся к A3 и отдельному разрешению на публикацию.

CGO собирается на самой старой заявленной baseline Ubuntu22.04 с pinned Go1.26.6/Node22.21.1. Docker — опция машины сборки, не VPS dependency. CI использует ту же baseline и больше не выполняет npm ci дважды. Дистрибутивы/CPU/kernel и systemd проверяются отдельно; byte-for-byte reproducibility не заявлена.

Build metadata явно показывает commit/dirty и source manifest hash; экспортируется полный source snapshot, включая dirty tree, без выдачи его за чистый commit. Финальные documentation-only commits не приписываются уже проверенному бинарнику. Подписан конкретный локальный предварительный asset, не опубликованный production Release.

Для старой обвязки выбран документированный путь clean install + encrypted restore. In-place OS/units/helper migration не автоматизирована: её нельзя подменять core-only update или повторным install.sh. Новый полноценный мигратор/rollback требует отдельной VM-приёмки.
