# Устройства прямых нод — 2026-11-06

Source/control-plane PASS в указанном scope; systemd/VPS/VPN/фактический disconnect NOT RUN.

- TypeScript/Vite build PASS,142 modules. В DevicesDialog только remote_pending notice и bounded polling пока окно открыто; не новая layout-матрица.
- `go test -race ./internal/httpapi ./internal/store ./internal/app ./internal/noderuntime ./internal/nodeagent -run 'TestRemoteDevice|TestDevice|TestNodeDeployment|TestCascadeCapability'`: PASS, HTTP1.076s/store1.027s/app1.096s/helper1.013s/agent1.016s.
- После v11: `go test -race ./internal/store ./internal/app ./internal/httpapi ./internal/noderuntime ./internal/nodeagent ./internal/backup -run 'TestRemoteDevice|TestDevice|TestNodeDeployment|TestCascadeCapability|TestIndependent|TestFirstProtocol'`: PASS, включая legacy device gates и v11 encryption roundtrip.
- Дополнительный restore regression обнаружил отсутствие node_control rows после пересоздания node identities. Исправлено INSERT inert control в той же транзакции, assertion не ослаблен. Финальный `go test -race ./internal/httpapi ./internal/store -run 'TestRemoteDevice|TestPortable|TestNodeDeployment'` — PASS.
- После переноса expensive package read за unchanged-generation fast path: `go test -race ./internal/httpapi -run '^TestRemoteDevice'` — PASS,1.066s; сохранён тот же credential/no-op/restore контракт.
- Backup targeted suite: `go test -race ./internal/backup ./internal/httpapi ./internal/store -run 'TestRemoteDevice|TestBackup|TestPortable|TestDevice'`: backup/store PASS; единственная первоначальная HTTP ошибка — missing inert node_control, исправлена указанным финальным прогоном.

Покрыт один реальный App/SQLite/HTTP поток с двумя агентскими bearer: create device → оба poll → helper validation нового kind → ACK одного не разрешает экспорт → ACK обоих выдаёт две прямые ссылки с одним device UUID. Старый общий UUID отсутствует в runtime payload. Изменение limit без изменения config не создаёт новый operation/restart. Revoke → экспорт404 → следующие payload без device и common UUID → ACK обоих закрывает remote_pending. Offline/withdrawn helper не мешает записать revoke, но запрещает новый credential. Finite package gate остаётся закрытым. Snapshot v11 валиден, restore не реактивирует revoke/старую публикацию/ACK.

ACK и helper execution в этом сценарии fixtures: это **не сетевой revoke, не device attestation, не remote traffic accounting**. Каскадные ноды/конечные пакеты не поддержаны этим блоком. A2 Release не пересобран; нужен новый согласованный агент/helper. [ADR-018](../../adr/018-direct-node-device-credentials.md).
