# Диагностика движков и уточнение UI

- `frontend: npm run build` — PASS (TypeScript + Vite).
- Linux existing image `arcnet-remote-tests:20261106`, offline: `go test ./internal/engines ./internal/httpapi -run 'TestApply|TestOperationError|TestActivation' -count=1` — PASS.
- Новые регрессии: безопасный текст с сохранением error cause; отклонённый validator не меняет прежний WG-файл; HTTP коды этапов без секретов; внешний xray rollback имеет приоритет.
- `git diff --check` — PASS.
- Browser / VPS / WG handshake / XHTTP handshake / UFW rollback — NOT RUN в этом блоке. Нет нового релиза или установки.
