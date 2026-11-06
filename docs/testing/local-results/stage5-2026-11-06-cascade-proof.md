# Внешний runner и publication — 2026-11-06

Реализованы CLI/UI/grant/report/publish/subscription gate. Это код/fixture validation, не реальная внешняя приёмка.

- `npm run build --prefix frontend`: TypeScript/Vite PASS,141 modules.
- `go test -race ./internal/store ./internal/nodeprobe ./internal/httpapi ./internal/app ./cmd/arcvpn -run 'TestCascade|TestNodeProbe|TestParseLink'`: PASS, store1.087s/nodeprobe1.012s/httpapi1.085s/app1.045s/cmd1.034s.
- Покрыты обе revision/digest, one-time token/replay/expiry, неправильный IP/отсутствующий DNS, отдельный publish, expiry/rollback прекращают выдачу, auth/CSRF, стабильный existing UUID/subscription/entry URI. DoH/egress responses в unit fake RoundTripper, ACK в API fixture синтетический.
- `ARCVPN_TEST_XRAY=/fixtures/xray go test -v ./internal/xray ./internal/noderuntime -run 'TestRealCascade|TestCascade'`: PASS, xray2.355s/helper0.028s. Настоящий pinned Xray26.10.31 принял усиленные production-конфиги finalRules; loopback два hop/fail-closed прошёл (тестовый plain VLESS, не REALITY/VPS).
- Реальный независимый runner/HTTPS IP/DoH через два VPS не запускались. DoH roundtrip не назван проверкой всех DNS leaks клиента.
- Визуальная браузерная приёмка сохранённого UI пока открыта; frontend build не заменяет её. A2 prerelease не пересобирался. Остальные пять больших функций остаются в работе, не объявлены готовыми.
