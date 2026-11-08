# Flow, смена ключей и Hysteria2 — 8 ноября 2026

Только локальные исходники; без релиза и изменений VPS. Рабочая копия с накопленными изменениями v7, не immutable release snapshot.

PASS, Linux Docker image `arcnet-remote-tests:20261106`:

```sh
go test ./internal/store ./internal/backup ./internal/app ./internal/httpapi ./internal/xray -run 'KeyRotation|StableExport|FlowAndSalamander|ServerOptionsValidation|HysteriaSalamander|VLESSFlow' -count=1 -timeout 60s
go test -race ./internal/httpapi -run 'KeyRotation|StableExport' -count=1 -timeout 60s
```

Проверены: flow runtime/URI и device clients; REALITY/XHTTP/WG rotation; stale audience/replay/CSRF; сохранение user identity; отказ apply и компенсация; публикация ответа, пересекающего смену ключей; busy503; auto Salamander secret/runtime JSON/URI; backup18 roundtrip и отказ новых полей под старой версией.

Frontend: `npm test` —28/28 PASS; `npm run build` — TypeScript/Vite PASS,221 модуль. `git diff --check` проверяется после документации. Логи текущих целевых запусков — task-owned `/private/tmp/arcnet-rotation-{check,race,frontend-tests}.log`.

Ранее в этом блоке проходили затронутые ServerOptions/ClientOptions/ConnectionEdit тесты шести пакетов, включая protocols. Повтор неизменённых широких наборов не выполнялся. Два финальных Go запуска повторены из-за потери результатов перед сокращением контекста, не из-за обнаруженной ошибки.

NOT RUN: настоящий Xray/WireGuard/Hysteria handshake, systemd/изолированная VM rollback/VPS, браузерная визуальная/keyboard/mobile приёмка (ограничение Work settings). Подмена процессов в fixtures проверяет source contracts, не сеть. UI review18394 — синтетические данные; его synthetic Xray не реализует keygen и не подтверждает REALITY rotation, для неё используются отдельные тестовые пары. WG/REALITY смена ключей требует обновления клиентского профиля. Backup18 не читается старыми binaries.
