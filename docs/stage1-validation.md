# Этап 1 — протокол локальной проверки

Дата: 2026-11-04. Рабочая версия исходников: **0.3.3-stage1-source-candidate**. Статус: **`IMPLEMENTING / EXTERNAL_PENDING`**, НЕ `LOCAL_PASS` и НЕ `ACCEPTED`.

## Реально выполнено в данном рабочем сеансе

| Проверка | Итог | Пояснение |
|---|---|---|
| `go test ./...` | PASS | Все пакеты, включая HTTP SPA и обработку ошибок |
| `go vet ./...` | PASS | Статический анализ Go |
| `go test -race` для `auth`, `backup`, `httpapi`, `logging`, `nodeagent`, `store`, `xray`, `qrcode` | PASS | Детектор гонок в критических пакетах |
| `bash -n` для основных установщиков/сборки | PASS | Синтаксис, не проверка выполнения на VPS |
| TypeScript AST parse | PASS | Синтаксическая проверка файлов TS/TSX, **не** typecheck |
| `python -m py_compile tests/react_e2e.py` | PASS | Синтаксис автоматизированного E2E, **не** запуск браузера |
| `go build -tags arcvpn_embed_react` + `frontend-check` | PASS на синтетическом fixture | Проверен **механизм** Go embed. Временные файлы удалены; не является React production build |
| SPA-маршруты и отсутствие JS/CSS/симлинков | PASS | Go HTTP-тест с синтетическим bundle, не React E2E |
| `npm ci`, `tsc -b`, `vite build` | BLOCKED | npm registry недоступен (DNS `EAI_AGAIN`), `package-lock.json` отсутствует |
| Playwright E2E реального React | NOT RUN | Нет React bundle; Chromium здесь блокирует навигацию даже к loopback (`ERR_BLOCKED_BY_ADMINISTRATOR`) |
| VPS: ACME, systemd, UFW, Xray handshake | NOT RUN | Нет тестовой внешней VM/VPS |

## Что изменено

1. `frontend/src/shared/api.ts`: 401 отзывается в UI даже при не-JSON ответе сервера, включена защита от HTTP-редиректов.
   Go API возвращает обезличенные ошибки для неожиданных сбоев Xray/SQLite, не раскрывая тексты системных команд в браузер.
2. `frontend/src/app/Auth.tsx`: периодическое обновление сессии с проверкой при фокусе.
3. `frontend/src/app/PageErrorBoundary.tsx`: ошибка рендера/загрузки страницы вместо пустой панели, с ручным повтором.
4. `frontend/src/pages/Backup.tsx`: после восстановления сбрасываются устаревшие данные React Query, уточнён объём текущего экспорта.
5. `frontend/src/pages/Routing.tsx`: схема только при реально сохранённой конфигурации; не выдаёт макет за активный VPN.
6. `frontend/src/app/Layout.tsx` и `frontend/src/style.css`: доступная навигация, ссылка «Перейти к содержимому», мобильная светлая тема, reduced motion.
7. `web/react_embedded.go`, `web/react_unavailable.go`: embedded Vite bundle только с release build tag; source/test binaries не скрывают его отсутствие.
8. `internal/httpapi/httpapi.go`: проверка собранного фронтенда, JS/CSS, отклонение symlink; SPA HTML не кэшируется.
9. `cmd/arcvpn/main.go`: приоритет embedded React; `serve --web-root` — только для разработки; `frontend-check` для preflight.
10. `scripts/build-release.sh`, `install.sh`: релизный Go-бинарник получает встроенный React; проверка наличия React до развёртывания сервисов, зависимость от runtime CGO библиотек учтена.
11. `tests/react_e2e.py` и `.github/workflows/ci.yml`: готовые сценарии для реального браузера и удалённого CI.

## Что требуется для `LOCAL_PASS` (Codex/CI с доступом к npm)

1. В `frontend/` один раз создать `package-lock.json`: `npm install --package-lock-only --ignore-scripts --no-audit --no-fund`, зафиксировать файл.
2. Запустить `./scripts/build-release.sh` в корне. Он выполнит `npm ci`, `tsc`, Vite, Go-тесты, CGO Linux build с встроенным React и `frontend-check`.
3. Запустить `python3 tests/react_e2e.py` с установленным Playwright Chromium. Скрипт проверяет реальный встроенный React + Go API: login, все маршруты, ноды, ошибки, cookie-тему и адаптивы 320/390/768/1024/1440.
4. При ошибках исправить код и повторить **оба** сценария. Сохранить скриншоты без секретов и отчёт о реальном прогоне.

## Что требуется для `ACCEPTED` (отдельный тестовый VPS)

Полная проверка `install.sh`, HTTPS-сертификата по IP, срока действия и renew, systemd, UFW/SSH без потери доступа, настоящего Xray + VLESS, сохранения последней рабочей конфигурации и замеров ресурсов. Порядок и форма результатов: [`testing/codex-handoff.md`](testing/codex-handoff.md).

**Результат текущей сессии:** расширена и локально проверена кодовая база, но полноценный этап 1 ещё не закрыт. Синтетические fixture не считаются реальной React-сборкой.
