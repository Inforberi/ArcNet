# ArcVPN 0.3.1: проверка первого этапа

**Статус:** частично завершённый этап, НЕ production. **Дата:** 2026-11-04.

## Изменения

- React: исправлены навигация, адаптивная сетка, 404, API-обработка 204/401, статусы/ошибки пользователей, ссылки и QR.
- Go: `/api/session` возвращает действительное имя администратора; `/api/overview` отображает локальный сервер плюс monitoring-агентов.
- Сборка: добавлен `scripts/build-release.sh`, устраняющий возможность назвать пакет release без React и Go; CGO оставлен включённым для SQLite и QR.
- Installer: `ARCVPN_REQUIRE_REACT=1` делает отсутствующий React-бандл явной ошибкой, preview legacy UI остается опцией.

## Проведённые проверки

| Команда/метод | Результат |
| --- | --- |
| `go test ./...` | PASS |
| `go vet ./...` | PASS |
| `bash -n install.sh scripts/build-release.sh` | PASS |
| `node --check web/app.js` | PASS |
| TypeScript `transpileModule` (15 файлов) | Без синтаксических ошибок; НЕ typecheck |
| `CGO_ENABLED=1 go build` для Linux amd64 | PASS, проверен запуск `version` |
| Chromium + реальный Go API через тестовый bridge, fallback UI | 1440 / 768 / 390 / 320: без console JS errors и горизонтального overflow |
| `npm install` | FAIL: EAI_AGAIN, DNS/registry недоступен |
| React production build / React E2E | НЕ выполнялись |
| Реальная установка, ACME, firewall, VPN tunnel | НЕ выполнялись |

## Как продолжить

В среде с интернетом и установленными Go 1.23+, gcc, SQLite/qrencode dev headers и Node.js:

```bash
./scripts/build-release.sh
```

После успешной сборки `install.sh` копирует `frontend/dist` в `/opt/arcvpn/www`; для обязательного React использовать `ARCVPN_REQUIRE_REACT=1`. Нельзя запускать production на рабочем VPN пока реальная установка и интеграция Xray не проверены.

## Оставшиеся задачи из этапа 1

Реальная React сборка, полный browser E2E, responsive React UI, установка и HTTPS на тестовой VM/VPS.
