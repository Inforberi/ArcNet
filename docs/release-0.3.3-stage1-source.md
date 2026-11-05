# ArcVPN 0.3.3 — доработка исходников этапа 1

Дата: 2026-11-04. **Статус: SOURCE CANDIDATE, НЕ PRODUCTION, этап 1 НЕ ПРИНЯТ.**

## В этом архиве

- Исправленные файлы React/TypeScript (сессии, 401, error boundaries, восстановление кэша, маршруты, мобильная тема, доступность).
- Go API, SQLite, Xray integration и HTTP handlers с проверкой Vite-бандла, SPA-маршрутов, отказом от симлинков и `no-store` для HTML.
- Go embed механизм: бинарник с `-tags arcvpn_embed_react` содержит реальный bundle после выполнения `scripts/build-release.sh`.
- CLI `frontend-check` и installer preflight; обычный dev-бинарник без React намеренно непригоден для установки.
- `.github/workflows/ci.yml`, `tests/react_e2e.py`, обновлённые `docs/ai/current-task.md`, `docs/stage1-validation.md`, `docs/testing/codex-handoff.md` и AGENTS.md.

## Что проверено здесь

`go test ./...`, `go vet ./...`, `go test -race` критических пакетов, сборка Linux x86_64 (dev), shell syntax, синтаксический разбор TS/TSX, раздача SPA с synthetic fixture, dry-run встраивания synthetic fixture с `frontend-check`.

Временные JS/CSS fixture удалены после проверки; **никакая synthetic сборка не выдаётся за React production**.

## Что осталось

- Генерация `frontend/package-lock.json` в доступной сетевой среде и фиксация lockfile.
- `npm ci`, TypeScript typecheck, Vite production build и настоящий Go release со встроенным React.
- Playwright E2E реального React + Go API: `python3 tests/react_e2e.py` на обычном Chromium с возможностью навигации.
- Чистый VPS: ACME HTTPS по IP, UFW, systemd, Xray connection, rollback и замеры нагрузки.

На текущей машине npm registry недоступен (`EAI_AGAIN`), Chromium запрещает навигацию (`ERR_BLOCKED_BY_ADMINISTRATOR`). **Нельзя объявлять LOCAL_PASS**.

## Следующие действия в Codex/CI

Из корня проекта на Linux x86_64 с сетевым доступом:

```bash
cd frontend
npm install --package-lock-only --ignore-scripts --no-audit --no-fund
cd ..
./scripts/build-release.sh
python3 tests/react_e2e.py
```

Прежде чем запускать E2E, установить Python Playwright и браузер Chromium. Если какой-либо тест не проходит — исправить код, повторить. Затем зафиксировать lockfile в git и выполнить внешний runbook на отдельном тестовом VPS.

Не устанавливать текущий архив поверх 3x-ui. `install.sh` отвергает dev-бинарник без настоящего embedded React.
