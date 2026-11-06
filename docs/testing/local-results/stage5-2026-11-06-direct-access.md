# Прямые подключения/QR — 2026-11-06

- `npm run build --prefix frontend`: TypeScript/Vite PASS,142 modules.
- `go test ./internal/httpapi -run 'TestDirectQR|TestDeviceDirectQR|TestIndependentProtocolAPI|TestIndependentWireGuard'`: PASS,0.022s. Второй URI выбирается вместо первого, SVG соответствует ему; stale selection409 без credential leakage; новый device QR session-only и отозванный device404. Прежние Hysteria/WireGuard exports проходят.
- `ARCVPN_FIRST_PROTOCOL_BROWSER=1 go test -count=1 -v ./internal/httpapi -run '^TestFirstProtocolBrowserFixture$'` + `tests/first_protocol_e2e.mjs` через установленный Chrome/Playwright: PASS,17.971s. Реальные React/Go/SQLite, Hysteria2-first/WireGuard/отдельная URI/кнопка копирования/device export и .conf,390/1440. Engines симулированы, это не VPN/телевизор/Happ/Streisand runtime test. Fixture сам завершён, постоянного сервера нет.

Старый E2E locator «Подписка / QR» заменён на пользовательское «Подключить». Никаких browser/npm installations или общей release/race матрицы. [Контракт](../../features/direct-client-access.md). A2 Release не пересобран, полный исходный архив передаётся отдельно.
