# Базовый UI и read-only firewall — 2026-11-07

## Реализовано в исходниках

- Пользователи: поиск имени/ID, фильтр состояния, срок в таблице; два основных действия вместо пяти. Карточка пользователя компонует обзор, существующие редакторы пакета/устройств, включение/отключение и подтверждаемое удаление. Нет новой схемы БД или подмены физических устройств количеством доступов. Срок вынесен из расширенных настроек.
- Общие раскрытия: chevron справа, клавиатура/focus, короткие анимации/reduced motion. Диалоги различают фон/внутренний клик/drag; закрытие изменённой формы требует подтверждения, busy блокирует закрытие. Редакторы подключения/пакета проверяют фактический diff; прочие формы используют консервативную отметку редактирования.
- SNI: не прошедшие проверку кандидаты не имеют бесполезной кнопки «Выбрать»; общая сводка при провале партии, ручной список через общий Advanced. Критерии пригодности не ослаблены.
- SVG графика заполняет полезную ширину; добавлены значения шкалы, выбранный замер привязан ко времени вместо сдвигающегося индекса. Это не новая серверная телеметрия.
- Подключена только точная root-команда `firewall status`. `allow/remove/enable` dispatch не подключён: обязательная изолированная VM rollback-приёмка ещё отсутствует. API сообщает `writable:false`; UI показывает найденное собственное правило, не выдаёт это за внешний доступ/закрытие порта. Предусмотрено обновление состояния.

## Фактические проверки

- Linux existing image `arcnet-remote-tests:20261106`, network none: `go test ./cmd/arcvpn ./internal/firewall ./internal/httpapi -run 'TestFirewall|TestParse|TestStage3AuthenticatedOperationsFailClosed' -count=1` PASS (0.007/0.003/0.023s). Включает отказ read-dispatch для команд изменения/лишних аргументов.
- Финальный Node22 TypeScript/Vite build PASS (Vite1.24s); dashboard/user-usage model tests — 4 PASS (117.60ms).
- CUA + настоящий React/HTTP/SQLite opt-in `TestOnlineUpdateBrowserFixture`: вход с synthetic учётной записью, поиск с пустым результатом/совпадением, карточка пользователя, переход к пакету, раскрытие и изменение checkbox/preview — PASS. При клике на фон изменённой формы показано confirm-предупреждение; cancel-путь инструмента не подтверждён (IAB не предоставил открытый JS dialog). Нативное заполнение datetime инструментом не обновило React-state; сохранение срока браузером не проверено.
- Mobile390: viewport/document/dialog width=390, горизонтального overflow нет. Скриншот `/private/tmp/arcnet-user-card.png` — desktop synthetic stand. После переноса срока наружу layout повторно не прогонялся; TS/Vite проверяет финальный код.
- Browser fixture штатно остановлен через свой endpoint, PASS 158.60s. Никаких настоящих engine/systemd/SSH/UFW изменений; это не VPN handshake или VM rollback PASS.

## Осталось

Оператор прислал диагностику: A7, panel/Xray/Hysteria/3x-ui active; listener ArcNet3443 и 3x-ui443, разрешения UFW3443 нет. Дана ручная команда allow3443/tcp и обновление подписки Happ; после открытия порта пользователь сообщил: до включения проверка отвечает, после включения VLESS в Happ сайты и проверка задержки не работают. VPN не принят. Запрошены bounded curl IPv4/IPv6 и безопасный текст ошибки клиента. CUA Termius снова отдаёт stale AX. На swe-promo сохранить 3x-ui/Hysteria2. Автоматическая смена портов/полный firewall/защита новых нод требуют изолированного rollback-стенда; текущий сервер с чужими службами его не заменяет. Индивидуальные назначения/массовые операции, имя пользователя/отдельное название подписки, расширение protocol settings по матрице совместимости и следующие серверные экраны этим блоком не завершены. Новый release/package пока не собирался и на VPS эти исходники не применялись.

Дополнительно: `TestFirewallReadStatusDoesNotPromisePortReachability` PASS (0.024s), реальный HTTP middleware + fake Unix helper проверяет точную read-команду и writable:false. Оператор подтвердил HTTPS IPv4=204, IPv6 timeout=3s. Причина VLESS всё ещё не доказана: запрошена конкретная ошибка Happ; IPv6 таймаут не объявляется причиной всего сбоя.

Внешняя bounded проверка build host: `curl -4 --resolve www.google.com:3443:82.148.226.252 https://www.google.com:3443/generate_204` с обычной TLS validation — HTTP204, TCP0.002786s/TLS0.727702s. Это REALITY fallback без клиентских credentials, не VPN handshake. Запрошены Wi-Fi comparison и jq public config summary. По официальному Xray sockopt AsIs TCP использует Go Happy Eyeballs с fallback, поэтому один IPv6 timeout не обосновывает принудительную смену IP strategy: https://github.com/XTLS/Xray-docs-next/blob/main/docs/config/transports/sockopt.md . Конфигурация Xray не менялась.
