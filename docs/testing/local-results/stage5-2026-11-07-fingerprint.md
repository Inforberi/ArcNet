# TLS fingerprint / XHTTP path — 2026-11-07

## Source checks

- Existing Linux image, network none: new xray fingerprint enum/default/link tests PASS; Store real SQLite save/list/get/replace + failed transaction rollback PASS; connection edit tests PASS; backup encrypt/decrypt baseline PASS. HTTP initially failed compile on a missing import, fixed before subsequent checks.
- `go test ./internal/httpapi -run 'TestFingerprint|TestConnectionManagement' -count=1` PASS0.050s: create TCP Firefox/XHTTP Safari; unsafe422, CSRF403, stale409; change FP with identical runtime bytes/private/public/ShortID; subscription includes both selected fp; edit path preserves preference and keys.
- `go test ./internal/backup -count=1` PASS1.069s. Historical v12 test now explicitly sets version12 after checking the current format, retaining remote-checkpoint legacy rejection coverage. New encrypted TCP/XHTTP preferences fixture initially used an invalid credential reference; corrected to the actual user's UUID, without weakening validator. New roundtrip/restore/rebind/legacy12/unsafe test PASS0.059s.
- Node22 TypeScript/Vite PASS1.18s. Final small copy-only change is followed by the release build.

## Browser

Real React/HTTP/SQLite opt-in fixture with synthetic credentials: login → existing VLESS → advanced → Chrome to Firefox → preview → apply → reopen → Firefox still selected; public key/ShortID shown, save disabled without changes. PASS. Screenshot `/private/tmp/arcnet-vless-fingerprint.png`; this is not the user's VPS. Fixture stopped via its endpoint, PASS106.71s. Engine restart unavailable on fixture; client-only branch intentionally does not invoke it.

## Limits

Real Happ/Streisand import/handshake and XHTTP path apply/rollback on a VM NOT RUN. VPS VLESS timeout remains unaccepted; changing fingerprint is not claimed as its fix. No SSH/UFW/units/cascade/engine changes. No DDL; existing settings keys used. New backup13 requires the new reader, old1–12 supported. All other advanced fields/per-user assignments/bulk/node work remain tracked in the main plan.
