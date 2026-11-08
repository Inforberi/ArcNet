# Локальные проверки пользовательских настроек

- Frontend `npm run build` (TS/Vite) — PASS.
- Linux existing image `arcnet-remote-tests:20261106`: `go test ./internal/store ./internal/app ./internal/protocols ./internal/xray ./internal/backup ./internal/httpapi` — PASS; самый длинный пакет HTTP API11.4s.
- Дополнительные targeted `go test -race` тех же пакетов без backup с `-run 'TestUserPreferences|TestUserAssignments' -count=1` — PASS.
- Новые тесты: SQLite CAS/сохранение identity/восстановление/запрет node enrollment; rollback runtime и metadata no restart; TCP/XHTTP isolation; отключение Hysteria/WG identities; backup14 сохраняет пустой deny-list; API anonymous/CSRF/stale и custom title.
- `git diff --check` — PASS.
- Локальная browser fixture обновлена, HTTP200 на18394. Только при `ARCVPN_UI_REVIEW=1` использует synthetic validator+SkipRestart и вымышленные Hysteria credentials в temporary dir. Это не Xray/systemd/VPN PASS.
- Browser interaction/визуальная приёмка не выполнены: ранее Work отключил local browser use; повторять заблокированный инструмент не стали. Настоящий VPN, удалённые ноды, VPS/systemd/UFW не проверялись и не менялись. Релиз не выпускался.
