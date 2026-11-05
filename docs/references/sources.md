# Источники и степень подтверждения

**Проверка публичных источников:** 2026-11-03. Ссылки — для будущих исполнителей, документация может измениться.

## Первичные источники

- 3x-ui README: https://github.com/MHSanaei/3x-ui — заявленные возможности протоколов, клиентов, multi-node, SQLite, API, monitoring.
- 3x-ui Clients: https://docs.sanaei.dev/docs/config/clients/ — поля пользователя, limits, renew, группы, online, subscriptions.
- 3x-ui Inbounds: https://docs.sanaei.dev/docs/config/inbounds/ — подключение, протокол и транспорт.
- 3x-ui Multi-node: https://docs.sanaei.dev/docs/operations/multi-node/ — другая полная панель как нода, management and hosts.
- 3x-ui Architecture: https://github.com/MHSanaei/3x-ui/blob/main/docs/architecture.md — публичное описание архитектуры (использовать как справочник, не копировать исходники).
- Remnawave quick start: https://docs.rw/learn-en/quick-start/ — Config Profiles, Hosts, Nodes, Squads.
- Remnawave Squads: https://docs.rw/learn-en/squads/ — назначение доступных Inbound группам.
- Xray-core: https://github.com/XTLS/Xray-core — отдельный VPN-движок.
- React Router: https://reactrouter.com/ — Data Mode.
- Tailwind: https://tailwindcss.com/docs — UI utilities/tokens.
- shadcn/ui: https://ui.shadcn.com/docs — компоненты как код, адаптация.
- Vite: https://vite.dev/guide/ — build статических assets.
- SQLite WAL: https://www.sqlite.org/wal.html — constraints одного writer.
- Podman Quadlet: https://docs.podman.io/en/latest/markdown/podman-systemd.unit.5.html — опциональное развёртывание.
- Let's Encrypt IP certificates: https://letsencrypt.org/2026/01/15/6day-and-ip-general-availability — короткоживущие сертификаты по публичному IP.
- Docker + firewall: https://docs.docker.com/engine/network/packet-filtering-firewalls/ — ограничения UFW при публикуемых контейнерных портах.

## Пользовательские материалы

- `docs/references/screenshots/dashboard.png` — реальный скрин 3x-ui dashboard (пользовательский визуальный эталон).
- `docs/references/screenshots/clients.png` — таблица пользователей; отображаемые имена и идентификаторы скрыты в копии для документации.
- `docs/references/screenshots/inbounds.png` — таблица входящих подключений.
- `docs/references/screenshots/client-form.png` — редактирование пользователя.
- `docs/references/auth-architecture-example.md` — пример правил Go-проекта, **не данные этого VPN-продукта**.

## Источники решения

Текущие требования в `docs/product/requirements.md` — пожелания пользователя плюс проектные выводы. Функция другого приложения, показанная в документации, **не означает**, что наша система её уже реализовала.

## Уточнения, перепроверенные 2026-11-03

- 3x-ui Multi-node: https://docs.sanaei.dev/docs/operations/multi-node/ — встроенная нода в 3x-ui требует второй панели, API/mTLS и Managed Hosts. Наша архитектура отдельного лёгкого агента — **новое требование**, а не готовая функция 3x-ui.
- 3x-ui API токены: https://github.com/MHSanaei/3x-ui/blob/main/docs/content/docs/en/reference/api/api-tokens.mdx — scoped API tokens и ограничения.
- Xray API: https://xtls.github.io/en/config/api.html — HandlerService, RoutingService, StatsService; не считать динамически изменяемыми *все* параметры JSON.
- Let's Encrypt certificates for IP: https://letsencrypt.org/2026/01/15/6day-and-ip-general-availability — 160 часов, нужны поддерживаемые ACME challenge и автоматическое продление.
- Docker/UFW: https://docs.docker.com/engine/network/packet-filtering-firewalls/ — опубликованные контейнерные порты могут обходить ожидаемые правила UFW.
- Примечание: описание пользовательской схемы маршрутов как два вида — продуктовая спецификация, не внешне подтверждённая функциональность любого из проектов.
