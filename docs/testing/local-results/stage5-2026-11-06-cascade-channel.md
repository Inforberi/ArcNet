# Каскадный канал/helper: проверки 2026-11-06

**CHANNEL_RENDERER_HELPER_LOCAL_PASS / MULTINODE_ORCHESTRATION_NOT_IMPLEMENTED / VPS_NOT_RUN.** [Контракт](../../adr/015-cascade-channel-and-node-operation.md). A2 prerelease не пересобран и не заменён этим блоком.

## Фактически выполнено

Linux arm64, существующий `arcvpn-local-test:20261105`, Go1.26.6, offline network namespace и native exec tmpfs. Никакие пользовательские службы/VM/VPS не менялись.

1. `go test ./internal/xray ./internal/noderuntime` — PASS после первоначальной реализации генератора/helper. Прежние direct и rollback тесты прошли.
2. Финальные затронутые сценарии:

```sh
ARCVPN_TEST_XRAY=/fixtures/xray go test -race -v ./internal/xray ./internal/noderuntime -run 'TestCascade|TestRealCascade'
```

Exit0: xray3.316s, noderuntime1.083s. Проверены независимые machine/user/device identities; сохранение разрешённых клиентов B и отсутствие disabled; default block/нет freedom на A; отказ для reused/private/self endpoints, conflicting port/key; privileged API/sockopt/proxySettings/routing bypass/duplicate JSON; новый kind/digest; rollback на прежнюю конфигурацию и lost-ACK replay без повторного restart. Последние helper-тесты используют fake systemctl.

3. Настоящий Xray **26.10.31 linux/arm64**, SHA256 бинарника `5939649960deaf535bcd5f7ce0467a2b1d63b72a2758a6f42eade714ae8d5328`: `run -test` для **неизменённых** production REALITY entry/exit — PASS. Первый запуск выявил несовместимый `::ffff:0:0/96` (Xray нормализует mapped IPv4 и отвергает mask96); правило удалено, затем затронутая проверка повторена успешно. Private mapped IPv4 обрабатывается IPv4-правилами; полномасштабная DNS/SSRF приёмка не заявлена.
4. Три настоящих Xray процесса (client/A/B) в loopback: HTTP body `through-two-nodes` принят через оба VLESS hop; после принудительной остановки B второй запрос завершился ошибкой и счётчик HTTP-сервера не увеличился. Процессы завершены fixture. Для этого сценария **только в тесте** заменены transport REALITY/Vision на plain TCP/VLESS, адреса/listen на loopback, снят private CIDR guard и добавлено freedom allow127.0.0.1/32. Это проверка двух hop/routing/fail-closed, **не REALITY handshake, не публичный VPN, не реальный exit IP**.
5. `gofmt` и `git diff --check` выполнены. Общая Go/frontend/release матрица не повторялась: frontend/API/DB не менялись.

## Не реализовано / не проверено

Нет LogicalEndpoint/user binding, encrypted storage machine identity, controller queue новой команды/capability negotiation, staged multi-node ACK/compensation/restart recovery, independent egress probe/publish. UI остаётся draft/simulation. Нет runtime failover/Geo/распределённых лимитов. Domain→private DNS и DNS policy требуют дальнейшей реализации; текущий буквальный CIDR guard не назван полноценной защитой. Systemd/receipt failure recovery/SSH/firewall/REALITY/VPS/TCP+UDP/accounting/512МиБ NOT RUN. Stage4/5 ACCEPTED не выставлен.
