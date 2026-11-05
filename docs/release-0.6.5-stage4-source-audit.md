# ArcVPN 0.6.5-stage4 — ревизия исходников и контроль этапа 4

**Дата:** 2026-11-04. **База:** ZIP 0.6.4; история 0.6.0–0.6.4 остаётся только для сопоставления изменений. **Статус:** `SOURCE_CHECKS_PASS / FRONTEND_BLOCKED / VPS_PENDING / STAGE4_NOT_ACCEPTED`. Это **полные исходники, не production-релиз**.

## Изменения

1. **Защита ключа конфигураций.** `internal/store/node_control.go` больше не открывает `node-control.key` через обычный `os.Open`; используются один FD, `O_NOFOLLOW|O_NONBLOCK`, проверка regular file/0600/размера 32 байта. Symlink/FIFO и права 0644 отклоняются. Тест — `TestNodeEncryptionKeyRejectsSymlinksAndNonRegularFiles`.
2. **Bounded retention.** В `store/node_control.go` удаляются старые terminal `node_operations`, оставляя 60 последних и не более 256 недавних, всегда сохраняя текущую ревизию и in-flight. В `store/node_bootstrap.go` аналогично очищается SSH журнал, оставляя минимум 50 последних, до 512 недавних, все in-flight и оригинальные установочные marker IDs действующих нод. SQL cleanup атомарен с созданием операции; тесты сохраняют текущие и чужие записи.
3. **Поддержка IPv6 без ослабления SSRF.** `ResolvePublic` проверяет корректный IPv6 до проверки DNS-хоста, запрещает private/link-local/documentation/mapped и зоны. `scp` использует `[IPv6]` для remote destination, иначе колоны ошибочно интерпретировались бы как разделитель пути. Тесты — `TestResolvePublicIPv6Literal`, `TestSCPDestinationWithPublicIP`.
4. **Устойчивость к обрыву контроллера.** `nodeagent.Run` использует экспоненциальную задержку 20s → 40s → 80s → 160s → 300s (плюс ограниченный jitter до потолка), повторный вызов не накладывается на предыдущий. При восстановлении связи интервал 20s. Внешний Xray не трогается. Проверяется `TestControlPlaneBackoffIsBoundedAndResets`.
5. **Релизная воспроизводимость.** `.github/workflows/ci.yml` требует **закоммиченный и проверенный** `frontend/package-lock.json`, а не генерирует его в CI на лету, скрывая незафиксированные зависимости. `nodeagent.Version`, `build-release.sh`, `verify-stage4-local.sh` согласованы на `0.6.5-stage4`. Релиз заблокирован без настоящего lockfile.

## Политика security/recovery

- Постоянный агент не умеет запускать произвольные shell-команды: только `apply_xray_v1` через ограниченный helper. Bootstrap SSH и upgrade — отдельные действия администратора с подписанным пакетом и проверенным fingerprint.
- `published=false` сохраняется до независимого подтверждённого VPN-probe и явных правил публикации. `heartbeat` и `systemd active` не доказывают data-plane.
- AES key исключён из переносимого backup: восстановление на другом контроллере требует отдельной перепривязки, а не безусловного импорта node credentials.
- TLS сервер + bearer ноды **не mTLS**. Компрометация bearer требует revoke/rebind, это документированный residual risk.
- Удаление/upgrade на реальном VPS, systemd и Xray не тестировались в этом контейнере, rollback по исходникам не гарантирует восстановления после обрыва питания.

## Фактические проверки и остатки

- Процедура: `bash scripts/verify-stage4-local.sh --source`; статус и необрезанный вывод — в `docs/testing/local-results/stage4-2026-11-04-0.6.5.*`.
- Go tests, vet, targeted race, CLI build, Bash/Python синтаксис: см. отдельный фактический протокол; синтаксический TS parse — это только синтаксис.
- `npm install --package-lock-only --ignore-scripts --no-audit --no-fund --fetch-retries=0` в этом окружении вернул `EAI_AGAIN` для registry.npmjs.org; lockfile не получен. Не генерировать поддельный.
- Не запускались: `npm ci`, TypeScript typecheck, Vite production, embedded frontend Go build, browser E2E, настоящие systemd/VPS/Xray/VLESS handshake.
- По каждому ID и разнице между реализованным, частичным и заблокированным: [матрица](planning/stage4-traceability-0.6.5.md). Для Codex/VPS: [внешняя приёмка](testing/stage4-codex-handoff.md).

**Приёмка:** нельзя выставить `LOCAL_PASS`, если обязательные frontend-проверки не прошли. Нельзя выставить `STAGE4_ACCEPTED`, пока реальные VPS и клиент не продемонстрировали весь жизненный цикл. Это не подэтап, а единое требование окончательной сдачи этапа 4.
