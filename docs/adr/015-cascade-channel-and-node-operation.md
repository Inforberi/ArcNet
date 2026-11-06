# ADR-015: отдельный канал каскада и операция node-helper

2026-11-06. Первый кодовый блок каскадов, **не многоузловой apply/publish**.

## Решение

`internal/xray/cascade.go` генерирует пару полных TCP VLESS/REALITY конфигураций из существующих профилей A/B и их пользователей. На B добавляется отдельный listener с новым machine UUID, X25519 keypair и short ID; пользовательские/device/enrollment credentials не используются как machine identity. Прежний listener B и его пользователи сохраняются. Требуются различные публичные IP, включённые профили и отдельный порт1024–65535. География не участвует в выборе направления. Конфигурации секретны; machine DTO не сериализует ключи, `String()` редактирует содержимое.

На A default outbound — block, маршрутизируется только известный inbound в единственный межнодовый outbound. Freedom/fallback/balancer отсутствуют. На B default тоже block, известные inbound направлены в direct. CIDR-фильтр блокирует буквальные приватные/служебные назначения; Geo-файлы не требуются. **Это не полноценная DNS/SSRF policy:** `AsIs` сохраняет доменные имена; закрытие доменных назначений, разрешающихся в private IP, и осмысленная DNS-маршрутизация остаются отдельным незавершённым gate до публикации.

`internal/noderuntime/cascade.go` — отдельный allowlist для `apply_cascade_xray_v1`, protocol1. Проверяются размер/JSON duplicate keys/depth, каждый listener прежним валидатором и дополнительными проверками keypair/UUID/target/портов/независимости, затем фиксированные outbounds/routing. Произвольные routes/API/paths/sockopt/proxySettings не допускаются. `apply_xray_v1` остаётся direct-only; старый агент/helper неизвестную новую операцию отклонит. Само наличие protocol1 не означает поддержку каскада: controller **не выдаёт** новую команду до отдельного capability/deployment gate.

Новый kind использует существующий Installer: Xray run-test → atomic replace → restart/active → root receipt. Повтор operation ID/revision/digest читает receipt; при restart failure возвращает прежний config. Active/receipt не доказывают VPN. Ошибка записи receipt после apply остаётся неопределённым результатом, требующим reconciliation, не успешным ACK. Тест rollback использует fake systemctl, это не VM-приёмка.

Формат [проверен на официальном Xray v26.10.31](https://github.com/XTLS/Xray-core/tree/v26.10.31/infra/conf), но pin протокола и реальные VPS должны приниматься отдельно. [Фактические результаты](../testing/local-results/stage5-2026-11-06-cascade-channel.md).

## Следующая обязательная часть

Стабильный LogicalEndpoint/user binding; encrypted persistence и ротация machine identity; desired/applied/last-good snapshots; persistent multi-node journal с подготовкой B перед A, CAS/ACK/reconcile и компенсацией; node capability gate; сохранение устройств/лимитов при изменениях; независимая revision-bound egress/DNS/UDP probe; отдельная publication revision. Генератор не подключён к UI/apply/poll и не меняет существующие черновики или подписки. Firewall автоматически не меняется. Не применять пару конфигов вручную к работающей панели вместо отсутствующей оркестрации.
