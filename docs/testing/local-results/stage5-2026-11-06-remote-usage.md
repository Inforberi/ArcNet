# Прямой remote TCP accounting — 2026-11-06

Исходники после A3, не опубликованный installer. Не выдавать этот блок за весь запрошенный готовый продукт или конечные распределённые пакеты.

## Выполнено

- Native Go: `go test ./internal/usage ./internal/store ./internal/xray ./internal/backup` в составе первого прогона изменённых пакетов — PASS. Включены Counter.Source, суммирование/backup v12 и совместимость v11. Native HTTP link недоступен из-за Linux-only libqrencode; native socket fixtures — ограничения sandbox/длины Unix path. Это не ошибки, замаскированные PASS.
- Linux, reusable `arcnet-remote-tests:20261106` (Go1.26): `go test ./internal/noderuntime ./internal/nodeagent ./internal/app ./internal/httpapi` и `go test ./cmd/arcvpn -run '^$'` — PASS. Последний шаг — только compile CLI, не запуск всех CLI tests.
- После исправления совместимости с cascade lease/restore kind: `go test -race ./internal/httpapi ./internal/store ./internal/app -run 'TestRemoteUsage|TestCascadeUI|TestUsage|TestPackage|TestPortable'` — PASS. Проверены параллельные повторы от двух нод, суммирование с local, чужой bearer/user, stale revision, регрессия, reset/high-water и каскадный rollback в accounting-конфигурацию.
- Agent bridge test обнаружил и исправил два wire-дефекта: optional Config в read-only helper query (`null` раньше не проходил ValidateRequest), повторную JSON-сериализацию report. `go test -race ./internal/nodeagent ./internal/noderuntime -run 'TestAgentUsage|TestUnixHelper|TestNodeStatistics|Test.*Capab'` — PASS после исправлений.
- `go vet ./internal/usage ./internal/store ./internal/xray ./internal/noderuntime ./internal/nodeagent ./internal/app ./internal/httpapi ./internal/backup` — PASS.

## Не проверено и следующий шаг

Новый настоящий remote StatsService/helper под отдельными systemd UID, TCP/REALITY handshake, окончание текущей сессии при revoke/лимите, restart/reboot/restore и конечные пакеты: NOT RUN. API/agent ACK и counters в тестах синтетические; нет VPN PASS. Новый frontend не менялся, его неизменённый A3 build используется повторно. Full race/новый installer/VPS-приёмка не запускались.

Пользователь предоставил VPS через Termius; root-login и read-only preflight подтверждены. Ubuntu26.04 LTS/x86_64, RAM3911МиБ, свободно5,5ГБ. Уже работают 3x-ui и Xray; пользователь выбрал сохранить их и проверять ArcVPN рядом. Ничего не установлено/не удалено, SSH/UFW/службы не изменены. Нужны изолированные ArcVPN пути/units, проверка свободных портов, snapshot/rollback перед изменениями. Одна машина не доказывает двухнодовый каскад. Доступ/адреса/credentials не включать в публичные документы.
