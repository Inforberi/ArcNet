# Независимый первый протокол — локальные проверки 2026-11-06

Объём: [ADR-012](../../adr/012-independent-first-local-protocol.md). Source/API/UI проверки, **не VPS/VPN acceptance**. Новых зависимостей, DB schema и privileged RPC нет.

## Go

Linux arm64, Docker image `arcvpn-local-test:20261105`, Go1.26.6, прежние SQLite/qrencode. Рабочая копия read-only, отдельный GOCACHE. Проверены затронутые пакеты, не полный release pipeline:

```sh
go test ./internal/app ./internal/store ./internal/backup ./internal/httpapi ./internal/xray
go vet ./internal/app ./internal/store ./internal/backup ./internal/httpapi ./internal/xray
go test -race ./internal/app ./internal/store ./internal/backup ./internal/httpapi -run 'Test(AnyLocalProtocolFirstWithoutReality|FirstProtocolAddressAndPortRemainStableWhenAddingReality|IndependentRestoreFailureRecoversAuxiliaryOnlyHost|IndependentServerAddressAndAccountingSources|V10IndependentProtocolBackupAndRemap|IndependentProtocolAPIAndSubscription|IndependentWireGuardSubscriptionHasNoFabricatedURI)$'
```

Все — exit 0. Основные race: app3.601s, store1.041s, backup1.013s, httpapi1.049s. После них отдельно уточнено сохранение исходного представления IPv6 в identity (без скрытой смены строки адреса): `go test -race ./internal/store -run 'Test(LocalServerPreservesExistingIPv6Representation|IndependentServerAddressAndAccountingSources)$'` — PASS. Остальные последующие изменения — UI/test selectors и docs; финальный frontend build PASS.

Проверены первые XHTTP/Hysteria2/WireGuard; у auxiliary-only fixture физически удалён fake Xray executable. Создание профиля/пользователя, toggle/delete, устройства/revoke, экспорт и encrypted roundtrip/restore проходят без `config` Reality. XHTTP JSON содержит только XHTTP inbound. Проверены запрет удаления последнего профиля при пользователях, конфликт IP/TCP-порта при последующем Setup, сохранение UUID/token/keys, восстановление прежнего auxiliary-only состояния при отказе Xray validation во время restore. Freshness работает без отсутствующего Xray и требует его после включения. Backup v10 remap сохраняет секреты/исходный снимок и меняет Hysteria IP SNI; смешанные адреса и новая capability под v1–v9 отклоняются. HTTP CSRF, подписки/connections/export и отсутствие выдуманной URI/прямого QR для WG-only проверены.

Во время реализации старые negative/version fixtures скорректированы: v2 по-прежнему запрещает отсутствие VPN, v9 проверяет свой явный номер. Удалён неиспользуемый import после удаления жёсткой связи экспорта с xray.Link. Существующие негативные security assertions не ослаблялись.

## Frontend / настоящий UI/API

`npm --prefix frontend run build` — TypeScript/Vite PASS, 140 modules. Node22.21.1, прежний lockfile. Проверка обнаружила отсутствующее поле `links` в TypeScript Subscription: DTO дополнен существующим API-полем, итоговый build PASS.

Opt-in `ARCVPN_FIRST_PROTOCOL_BROWSER=1 go test ./internal/httpapi -run '^TestFirstProtocolBrowserFixture$' -count=1 -v` поднимает отдельный loopback fixture на18392. `tests/first_protocol_e2e.mjs` запускается Playwright/Chromium из установленного временного окружения с `ARCVPN_E2E_URL=http://127.0.0.1:18392`, `ARCVPN_CHROMIUM` и `NODE_PATH` на Playwright. Настоящие auth/CSRF/HTTP/SQLite, **движки simulated/SkipRestart**, без перехвата API. Публичного сервера/production credentials нет.

Browser сценарий: Hysteria2 первым из UI, WireGuard вторым, `configured:false` для TCP REALITY, создание пользователя, только реальные виды подключений, URI Hysteria и отдельная загрузка WG device `.conf`. Общая выдача доступа не подписана как «VLESS». Проверены390/1440px, отсутствие JS errors/overflow; скриншоты `tests/screenshots/first-protocol/`, мобильный просмотрен визуально. Опциональная TCP-форма открывается отдельной кнопкой и не навязывается после создания Hysteria/WG.

Первая попытка ожидания `networkidle` завершилась timeout из-за live-страницы; заменена на `domcontentloaded` и проверку нужных элементов. Повторный браузерный прогон PASS. Старый monitoring E2E selector адаптирован к опциональному открытию TCP и нескольким независимым формам; полный monitoring/regression suite этим блоком повторно не запускался.

## Внешние gates

Никаких реальных Xray/QUIC/WG handshake, public VPS, systemd/ACME, NAT/root-helper rollback или нагрузки512МиБ/1vCPU в этом блоке не запускалось. Сохранение/синтетический apply не доказывают доступность VPN. Установщик всё ещё имеет собственный состав runtime dependencies; его оптимизация не входит в этот блок. Remote nodes, distributed quotas, Geo, каскад/runtime/failover/publish остаются открытыми.
