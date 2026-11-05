# Приёмка ArcVPN 0.6.4-stage4 — обязательные сценарии после локального security hardening

**Не выполнено на реальном VPS.** Удалённый тестер использует полный source ZIP версии 0.6.4, проверяет SHA-256 архива, затем запускает исходную матрицу EXT4-01…29 ниже. Дополнительно **обязательно**:

- `EXT4-30` Выключить systemd Xray после успешной конфигурации, повторно доставить тот же `operation_id/revision/digest`; helper должен вернуть FAIL, не `applied`, до исправления сервиса.
- `EXT4-31` Повредить либо заменить символьной ссылкой root receipt и xray config в изолированной VM; попытка apply должна останавливаться без ложного ACK. Снимок VM обязателен.
- `EXT4-32` Имитировать отказ второго `useradd` после создания первого и отдельно гонку уже существующего `arcvpn-xray`; компенсация удаляет только аккаунт, созданный данной установкой.
- `EXT4-33` Невалидный контроллер-origin, host с `'`/`;`, порт за диапазоном, подменённый signed ZIP, небезопасные права signing private key; установка/CLI должны отказывать до изменений, shell-команды из UI должны оставаться корректно процитированы.
- `EXT4-34` `npm ci`, TypeScript typecheck, production React build, embedded Go binary, Playwright/Chromium на 320/390/768/1024/1440; прикрепить реальные диагностические сообщения и исправить обнаруженные ошибки.

Отчёты только в `docs/testing/external-results/` после **реального запуска**, с exit codes, timestamp, OS/Xray versions, средой тестирования и очищенными логами. `LOCAL_PASS`/`ACCEPTED` до этих испытаний запрещены. [Локальные факты](../testing/local-results/stage4-2026-11-04-0.6.4.md).

## Исторический набор внешних проверок 0.6.3 (NOT RUN)

Для полного source ZIP 0.6.3 необходим **независимый** клиент (не контроллер и не удалённая нода), реальный Xray, действительная VLESS/REALITY конфигурация и HTTPS egress. Только подготовленные/фактически выполненные результаты записывать в `docs/testing/external-results/`. [Исходники и ограничения](../release-0.6.3-stage4-continuation.md).

| ID | Сценарий | Критерий приёмки |
|---|---|---|
| EXT4-25 | С внешней клиентской сети запустить `arcvpn node-probe` и доверенный Xray по private 0600 VLESS link (real node, real Reality) | Xray `run -test` PASS, TLS запрос через VLESS/REALITY HTTPS 204, реальный egress IP проверить независимо, протокол и version Xray в журнале; в панели report привязан к применённой ревизии; `published=false` |
| EXT4-26 | Невалидный SNI, public key/shortId, блокировка VPN-порта, выход Xray, исчезновение DNS/HTTPS target | `fail/unknown`, не `pass`; нет перехода на прямой маршрут, временные конфиги 0600 удаляются, секреты не утекли |
| EXT4-27 | Сгенерировать grant, отправить его повторно/одновременно, после смены ревизии/drain/upgrade/removal/истечения 5 минут | Старый токен отвергается, старый PASS не остаётся актуальным; после 30 минут `stale` (с управляемым clock test); новая публикация не происходит автоматически |
| EXT4-28 | Browser UI выдачи grant, TTL и скрытие токена, запись отчёта и отображение статусов в таблице/схеме на 320/390/768/1024/1440 | Настоящий `npm ci`, typecheck, React build и Playwright выполнены, ошибки и accessibility зафиксированы; bearer ни в URL, ни в argv, ни в логах |
| EXT4-29 | Угроза поддельного отчёта с украденным grant и перенос с клиентской машины | Зафиксировано, что bearer **сам по себе** не доказывает фактический запуск; нельзя писать hardware-attested. Сохранить внешние независимые evidence |

**Порядок:** сначала закрыть React build, затем clean VPS install / Xray, затем внешний клиентский probe и управляемые отказы, затем оформить acceptance. Сохранить commit или ZIP SHA-256, команды, выходные коды и очищенные логи. Продолжить историческую матрицу EXT4-01..24 ниже. Текущий статус каждого нового сценария — **NOT RUN**.

---

# Этап 4 — внешняя проверка Codex / VM (исходники 0.6.3)

**Статус 2026-11-04:** `EXTERNAL_PENDING / NOT RUN` — это задания на реальный стенд, **не** отчёт об успешных испытаниях. Текущая реализация частичная: SSH wizard, подписанная установка, managed uninstall/rebind/upgrade, Xray helper, node control и React TSX есть в исходниках; npm CI/frontend, VPS и handshake не проверены. См. [фактический handoff](../release-0.6.1-stage4-continuation.md), [операционные ограничения](../ops/stage4-nodes.md) и [чеклист](../planning/stage4-checklist.md). **Не запускать на рабочем 3x-ui/production VPS.**

Внешний Codex сначала должен закрыть React lockfile/build и browser E2E (`S4-01`, `S4-44`), затем проверить уже написанный CLI install/managed uninstall/rebind/upgrade на чистой VM и исправить реальные ошибки. Не помечать `EXT4-05/11/13/14` выполненными без фактических VM/E2E доказательств.

## Подготовка тестового стенда

- Контроллер A: отдельная Debian/Ubuntu VM/VPS с console recovery, snapshot, публичным HTTPS, установленным ArcVPN и проверенным комплектом бинарников/подписей.
- Нода B: **чистая** отдельная VM/VPS с отдельным публичным IP, SSH на настраиваемом порту, systemd, snapshot и console recovery.
- Клиент C: независимая сеть/устройство с поддерживаемым VPN-клиентом, чтобы проверить фактический входящий handshake, DNS, UDP/TCP и egress IP. Не заменять этот тест ping/heartbeat.
- Версии Go, Node, ОС, Xray, AWG, release SHA256 и commit SHA фиксировать в отчёте. Без Git commit SHA разрешено писать только идентификатор source archive/hash.
- Секреты SSH, enrollment, VPN, TOTP, private keys не попадут в отчёт/скриншоты/command line. Не запускать тесты на рабочем 3x-ui.

## Шаги и ожидаемые результаты

| ID | Что выполнить | Условие PASS |
|---|---|---|
| EXT4-01 | Чистый React build и реальный браузер | lockfile закоммичен, `npm ci`, `typecheck`, `build`, E2E успешно завершены |
| EXT4-02 | SSH-wizard: неверный host key, private IP policy, нет sudo, занят порт | Остановка до изменения ноды; нет утечки секретов |
| EXT4-03 | Read-only preflight на B + просмотр diff | Показаны OS/arch/resources/порты/сервисы/риски; пока не подтверждено, ничего не установлено |
| EXT4-04 | UI automatic install на чистый B, порт 443 | Identity + Xray + агент установлены, Xray слушает 443 от `arcvpn-xray` с CAP_NET_BIND_SERVICE; конфиг root-owned 0640, receipt root-only; no arbitrary shell RPC |
| EXT4-05 | Альтернативная одноразовая команда | Те же гарантии trust/secret handling, повтор не создаёт второй узел |
| EXT4-06 | Применить desired VPN revision | Atomic config, revision ACK, service healthy, внешнее клиентское подключение C работает |
| EXT4-07 | Подменить config/подпись, force service start failure | Подмена Ed25519 подписи и host-key блокируется; отказ restart откатывает last-good; квитанция `/var/lib/arcvpn-node-root/last-apply.json` недоступна аккаунту `arcvpn-node`; failed status и no publication |
| EXT4-08 | Остановить A / потерять сеть / перезапустить B | B продолжает обслуживать уже настроенный VPN, stale telemetry корректно отображается |
| EXT4-09 | Дубли/повторы/конкуренция/обрыв установки | При потерянном ACK Retry сохраняет прежний operation ID, helper идемпотентен; после восьми доставок состояние failed; не более двух одновременных SSH-установок; при обрыве возможен ручной console recovery |
| EXT4-10 | Ротация/отзыв одного node credential | Отозванная нода не управляется, другие ноды продолжают работать |
| EXT4-11 | Закрыть UI, перезапустить контроллер, сменить адрес | Установочная операция не теряется, процедура rebind документирована и проверена |
| EXT4-12 | UFW и чужие службы B | SSH сохраняется, меняются только ArcVPN-managed правила/ресурсы, rollback при сбое |
| EXT4-13 | Схема/таблица на 320/390/768/1024/1440 | Состояния совпадают, выбор и фильтры синхронизированы, keyboard и темы работают |
| EXT4-14 | Uninstall/revoke/replace B | Только подтверждённые управляемые ресурсы, нет уничтожения чужих сервисов или лишнего изменения firewall |

## Новые обязательные сценарии продолжения 0.6.1

| ID | Что проверить на отдельной VM | PASS только если |
|---|---|---|
| EXT4-15 | Signed upgrade 0.6.0→0.6.1, неверная подпись, downgrade, invalid Xray, fail restart | Невалидные запросы отвергнуты до изменений; успешный апгрейд даёт новый бинарник и restart active, отказ возвращает обе старые бинарные версии; внешний handshake после каждой ветви подтверждён |
| EXT4-16 | Detached Xray, повторный `purge`, чужие файлы в managed directory | `detach` удаляет agent/helper, но Xray и клиентский трафик продолжают работать; purge удаляет только managed; неожиданные файлы не удаляются рекурсивно; чужой systemd/SSH/UFW нетронут |
| EXT4-17 | Local `node-install` из независимо подписанного файла, неверный public key, потерянный enrollment reply | При неверном ключе нет установленных unit; токен не появляется в argv/journal, на ошибке без identity выполняется managed rollback; при неопределённой регистрации сохраняется материал для безопасного recovery |
| EXT4-18 | Rebind A→A2 с независимыми HTTPS PKI и токеном, отказ нового сервера | Меняется только agent identity, прежний Xray продолжает работу; при отказе новое identity не подменяет прежнее; устаревший агент без protocol-v1 не получает apply |
| EXT4-19 | Контроллер и нода перезагружаются между планом, утверждением и SSH upgrade; потеря SSH/питания в момент изменения | В журнале нет ложного `upgraded`, recover после power loss документирован отдельно, действительный state бинарников/systemd проверен, следов секретов нет |

Эти сценарии **НЕ запускались** в текущем окружении. Прежде чем отметить PASS, Codex обязан приложить exit codes и очищенные журналы. Простой ответ «проверено» или симуляция shell-синтаксиса не подходят.

## Как оформлять доказательства

После реального запуска создавать **отдельный** `docs/testing/external-results/stage4-YYYY-MM-DD.md` со следующими полями по каждому EXT4 ID:

- Git commit SHA или исходный ZIP SHA-256; versions, OS, architecture, network topology (без секретов).
- Фактическая команда/действие, expected, observed, exit code и sanitized log.
- `PASS` / `FAIL` / `BLOCKED` / `NOT RUN`, ссылка на артефакт, подтверждение теста с клиентской сети.
- Что изменилось в системе и как выполнен rollback/restore; известные ограничения.

Завершение **всего** этапа 4 требует одновременно `LOCAL_PASS` и внешних результатов без блокирующих failures; формулировка «Codex проверит позже» не является `ACCEPTED`.

## Обязательные замечания к исполнению

- На чистом VPS сверить SHA-256 release и **Ed25519 trust anchor** через независимый канал, проверить ownership системных каталогов до и после. Никогда не передавать SSH key/token в аргументах процесса или report.
- Проверить `sudo systemctl cat arcvpn-node-xray.service` и `sudo ss -lntp`, а также `sudo -u arcvpn-node test -w /var/lib/arcvpn-node-root` (должно отказать) и `sudo -u arcvpn-xray test -r /etc/arcvpn-node/xray.json` (должно работать после apply).
- Порт VLESS TCP может быть заблокирован UFW/облачным firewall: bootstrap их **не меняет**. При доступной provider console открыть только нужный VPN-порт вручную без риска потери SSH, затем проверить клиентом C.
- После отказа установки проверять отсутствие orphan systemd units и управляемых файлов, а также оставшиеся изменения `apt`: компенсация файлов/служб не отменяет изменения пакетов.
- Обязательно проверить ручной `node-register --token-stdin`: одна вставленная строка + Enter, Ctrl+D не требуется; токен однократный, за пределами TTL отвергается.
- `docs/testing/external-results/` должно содержать только реально проведённые проверки. Каждый непройденный пункт обозначать `FAIL`/`BLOCKED`/`NOT RUN`, не писать итоговый `ACCEPTED`.

## Дополнительные проверки для 0.6.2 — запускать на изолированной VM

| ID | Действие | Доказательство PASS |
|---|---|---|
| EXT4-20 | После контроллерной проверки Ed25519 signed bundle подменить копируемые `agent`/`xray` в user-owned SSH staging до root-копирования и перед подписанным upgrade | Невалидные байты **никогда не запускаются**; SHA-256 в root-owned paths отвергает подмену; старый Xray остаётся активен. Снять `sha256sum`, unit status, sanitised logs |
| EXT4-21 | Обрыв SSH stdout/коннекта после реального успешного install/upgrade/detach/purge, а также обрыв до завершения | Успех только при совпавших root marker/version/binary hash/service/managed state. На неопределённом результате — не успех, а корректная диагностика и console recovery |
| EXT4-22 | `curl` отсутствует, DNS контроллера недоступен, неправильный TLS CA, HTTP 302/401/503, UFW active, нестандартный SSH порт | Preflight не отключает TLS, не меняет UFW/SSH, не скрывает warnings; отсутствие HTTPS 204 останавливает install, отсутствие curl помечено `unverified`; cloud firewall проверить независимо |
| EXT4-23 | В UI пройти drain на работающей ноде, открыть ранее сохранённый client link, запросить новый диагностический link, затем resume | При drain новые ссылки HTTP 409 и published false; уже выданные ссылки **могут** работать (это не revoke); после resume ранее применённая конфигурация не меняется, внешний probe по-прежнему unknown |
| EXT4-24 | Прогнать актуальный `tests/react_e2e.py` на 320/390/768/1024/1440 после React production build | Реальная страница «Серверы», текущие кнопки, регистрация, drain, отсутствие секретов в браузерных логах/скриншотах, без переполнения экранов |

Эти сценарии **NOT RUN** в исходном окружении 0.6.2. Добавить результаты и коды выхода в `docs/testing/external-results/`, не заменять внешнее доказательство локальным shell/Go unit тестом.

## Дополнение 0.6.5 — обязательная окончательная приёмка (НЕ запускалась здесь)

**Исходники:** полный ZIP 0.6.5-stage4. Прежде чем проверять VPS, откройте `docs/ai/current-task.md`, `docs/planning/stage4-traceability-0.6.5.md`, `docs/testing/local-results/stage4-2026-11-04-0.6.5.md`. Доказательства по этапу 4 — реальные логи в `docs/testing/external-results/`, без токенов, VLESS URI и приватных ключей.

1. На доверенном **сетевом build runner** запустить `cd frontend && npm install --package-lock-only --ignore-scripts --no-audit --no-fund`, проверить resolved dependencies и зафиксировать `package-lock.json` в репозитории. Далее выполнить `npm ci`, `npm run typecheck`, `npm run build`, `scripts/build-release.sh`, `frontend-check` и `tests/react_e2e.py` на реальном Chromium для 320/390/768/1024/1440. Отдельно проверить ошибки управления нодами/повторные кнопки/keyboard навигацию. Если TypeScript/E2E падает — исправить исходники в этапе 4 и повторить.
2. На двух **изолированных** VM с snapshot/rescue консолью проверить root-owned install marker, права `node-control.key` (на контроллере), agent `node.json` (на ноде), helper socket, systemd service units, запрет неавторизованного Unix caller, custom SSH port/Ed25519 pinning, повторную установку/удаление/upgrade и отказ от чужих компонентов. Установить подтверждённый подписанный Xray и agent одной версии. **Не запускать на production.**
3. Отдельно испытать **публичный IPv6**: `Inspect`/preflight/SSH/SCP с подтверждённым fingerprint и IPv6-only VPS; проверить, что private/link-local IPv6 и IPv4-mapped addresses отвергаются. Проверить реальное восстановление после разрыва SSH во время upload/install/update, через provider console и readback marker/digest.
4. Отключить контроллер на время больше 5 минут: агент должен снижать частоту retry (bounded backoff), существующий VPN должен обслуживать egress, после восстановления новый poll должен продолжить работу без потерянной ревизии. Зафиксировать реальные timestamps и отсутствие одновременных agent apply.
5. Создать более 256 terminal control operations и более 512 bootstrap operations **в тестовой БД**; убедиться, что in-flight, оригинальный installed marker и журналы других нод сохранены, а older terminal entries очищены. Не делать это на production.
6. Настоящим Xray-клиентом через независимую сеть подтвердить handshake VLESS/REALITY + TLS HTTPS 204 через SOCKS, отрицательные сценарии и отсутствие direct fallback. `published=false` до внешней проверки; bearer grant без доказанного места запуска нельзя считать remote attestation.
7. Отдельно получить evidence AC из `docs/planning/acceptance.md`; **только при PASS** отметить `LOCAL_PASS` (frontend + локальные тесты) и `STAGE4_ACCEPTED` (real VPS + независимый клиент). В протокол записать дату, версии, SHA-256 сборок, коды ошибок и санитизированные exit statuses. При FAIL исправить код до перехода к этапу 5.
