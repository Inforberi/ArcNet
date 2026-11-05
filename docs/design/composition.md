# Component Composition и границы frontend

## Принцип

Небольшие компоненты с одной ответственностью; layout и поведение собираются композиционно. Бизнес-правила, авторизация и политика доступа остаются на Go-сервере, frontend отвечает за отображение, валидацию формы и пользовательские состояния.

## Пример API композиции (иллюстрация, не существующий код)

```tsx
<ServerCard>
  <ServerCard.Header>
    <ServerCard.Name />
    <ServerCard.Status />
  </ServerCard.Header>
  <ServerCard.Metrics />
  <ServerCard.Actions />
</ServerCard>
```

Для простого компонента не вводить compound API без нужды. Не создавать универсальный mega-component с props для всего приложения.

## Данные и состояния

- React Router Data Mode: URL, nested layouts, loader/action при уместности, обработка ошибок, lazy chunks.
- TanStack Query: server-state, invalidation, request cancellation, stale/retry policy.
- SSE: ограниченные агрегированные обновления статусов, backoff переподключения, очистка EventSource при unmount.
- URL отражает фильтры/пагинацию там, где полезно для шаринга и возврата.
- Действия с риском показывают optimistic/pending state осторожно: нельзя считать изменение UFW/Xray успешным до подтверждения backend.
- Статические файлы Vite встроены через `go:embed`; Go обслуживает SPA fallback только для UI-маршрутов, а API/статические файлы/404 не подменяются `index.html`.

## Структура (проектная)

`app/` (Router и providers) → `pages/` (сборка экранов) → `widgets/` (объединённые блоки) → `features/` (действия пользователя) → `entities/` (типизированные предметные view models) → `shared/` (UI и системные утилиты).

Не превращать это в жёсткую FSD-схему с пустыми директориями. Слои создаются по мере появления повторно используемого кода. Не класть Go domain logic в frontend.

## API-контракты

JSON-типы должны выводиться из описанного API (OpenAPI/generator после POC) или централизованно поддерживаться. Запрещены разные несогласованные типы одних сущностей на пяти страницах. Даты — ISO8601 в API, преобразование и временная зона в UI явно определены.
