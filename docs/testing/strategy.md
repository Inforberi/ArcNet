# Стратегия тестирования

1. **Go unit:** доменные политики/статусы, права, время/renewal, квоты, генератор конфигурации, failover logic.
2. **Repository SQLite:** настоящая disposable SQLite, migrations up/down, WAL concurrency, rollback и idempotency.
3. **HTTP:** auth/cookie/CSRF, validation, error mapping, permissions, rate limits, no secret logging.
4. **Xray integration:** реальные совместимые версии в изолированном стенде, config test, apply, rollback, traffic counters.
5. **System operations:** UFW/nftables и systemd только в VM/network namespace, никогда на хосте разработчика. Проверять сохранение SSH на виртуальном сервере.
6. **Agent integration:** mTLS, stale revisions, replay protection, node offline, reconnect, rollback/partial failures.
7. **Frontend:** компоненты и формы, fake API для UI, E2E с реальным backend для критических сценариев, keyboard/a11y.
8. **Performance:** фиксированный профиль, повторяемые замеры CPU/RAM, масштабирование до нескольких нод, нагрузка и длительные soak tests.
9. **Security:** threat model, secret leak checks, SSRF через installer, origin/CSRF, path traversal, port/credential escalation, TLS pin/trust.
10. **Recovery:** резервная копия, настоящий restore, отказ диска/соединения, недоступность панели и подписок.

Интеграционный `sandbox/` запускает тот же backend, не отдельную имитацию VPN-сервиса. Test fixtures — только синтетические. Go private tests могут жить рядом с кодом, внешние scenario tests — в локальных `tests/` подпакетах; проверять соответствующие команды, потому что `go test` одного пакета не запускает sibling test-пакет.

Каждый факт прохождения тестов сопровождается командой/окружением/результатом в current-task или CI; неподтверждённые проверки — «не запускались».

## Экономия времени без потери контроля

В каждом цикле выполнять быстрые проверки изменённых мест; для привилегий, ключей, экспорта, маршрутов и конфигурации VPN негативные тесты **не откладывать**. Долгие E2E/race/soak и реальные клиентские проверки выполнять на gate этапа или внешнем Codex-стенде. Ни mock, ни статус Go-process `running` не подтверждают качество пользовательского VPN.

См. [матрицу доступных проверок](../planning/execution-boundaries.md) и [операционный runbook для Codex](codex-handoff.md).
