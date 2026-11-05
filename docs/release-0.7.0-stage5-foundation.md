> Продолжение 2026-11-05: [редактор стрелок и backup v4](release-0.7.0-stage5-routing-ui.md), [реальные проверки](testing/local-results/stage5-2026-11-05-routing-ui.md). Ниже исходный foundation handoff; старое отсутствие frontend-сборки больше не является текущим статусом. Data-plane по-прежнему не реализован.

# ArcVPN 0.7.0-stage5-foundation — передача базы этапа 5

**Дата:** 2026-11-05  
**Основа:** подтверждённый SHA-256 исходного архива `arcvpn-0.6.5-stage4-complete-source-handoff.zip` (проверен перед правками).  
**Статус:** `STAGE5_DRAFT_CONTROL_IMPLEMENTED / SOURCE_TESTS_PASS / DATAPLANE_NOT_IMPLEMENTED / FRONTEND_BUILD_NOT_VERIFIED`.

## Что реально сделано

1. `internal/routes/policy.go` — typed draft-only `Policy`, `Edge`, ограничения на участников и резервные выходы, проверка направленного графа на циклы, в том числе через backup.
2. `internal/routes/simulate.go` — чистая тестовая модель direct/cascade/fail-closed/explicit fail-open/hysteresis/cooldown. `unknown` блокирует маршрут. Даже при здоровом выходе, отсутствующий/упавший вход не становится доступным. **Сэмплы вручную передаёт тестер**, результаты не доказывают VPN.
3. `internal/routes/diff.go` — воспроизводимый read-only diff черновиков.
4. `internal/store/routes.go` и `internal/store/store.go` — SQL `route_policies`, idempotent migration, ссылки на существующие `managed_nodes`, CAS-версии, транзакционный граф-чек и сохранение, удаление черновика.
5. `internal/httpapi/routes.go`, `httpapi.go` — `/api/routes`, preview, create/read/update/delete/simulate; все опасные методы требуют сессию+CSRF; никакого endpoint `activate`.
6. `frontend/src/pages/Routing.tsx` — конструктор и редактор черновика, preview/diff графа, таблица сохранённых политик, локальная симуляция отказа, явное предупреждение о неактивности; `frontend/src/shared/api.ts` поддерживает `PUT`.
7. Go unit/store/http тесты. Полный перечень команд и честный результат: `docs/testing/local-results/stage5-2026-11-05-foundation.md`.

## Что это НЕ делает

- **Не конфигурирует** Xray ни локально, ни на VPS и не создает межнодовых туннелей.
- **Не публикует** подписки и не переключает реальных пользователей.
- **Не измеряет** качество или адрес выхода, не осуществляет автоматический failover в runtime.
- Не даёт оснований говорить, что A→B или B→A работает на сети.
- Не завершает этап 4 и не является production-релизом.

Переход к движку должен быть защищён архитектурным gate: проверенная секретная межнодовая идентичность → staged apply → подтверждённый ACK **всех** нод → независимая probe с exit IP → лишь затем отдельное publish. При сбое на одном узле нужен журнал компенсационных операций. Сам факт «policy saved» не должен влиять на VPN.

## Почему не объявлено завершение этапа

Полный перечень — `docs/planning/stage5-checklist.md`; воспроизводимые внешние действия — `docs/testing/stage5-codex-handoff.md`. Наличие Go PASS не подтверждает ни React production build, ни живой VPN. Тесты в проекте не включают фактическое переключение из VLESS к другому VPS.
