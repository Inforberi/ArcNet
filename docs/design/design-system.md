# Дизайн-система: правила

## Основа

- React + TypeScript + Tailwind CSS.
- Выборочные shadcn/ui primitives как старт; адаптация компонентов в собственном `shared/ui`.
- CSS Variables — источник токенов; Tailwind utilities подключают эти значения.
- Две темы: dark по умолчанию, light опционально.
- Графики — ограниченная библиотека SVG/Canvas после сравнения размера и поведения; не устанавливать тяжёлый chart package для двух sparklines.
- Композиция компонентов; не делать `UniversalCard` с 40 boolean prop.

## Токены (проектные категории, конкретные значения после прототипа)

| Группа | Примеры |
|---|---|
| `surface` | app, panel, elevated, hover, active |
| `text` | primary, secondary, muted, inverted |
| `border` | subtle, default, focus, critical |
| `accent` | primary, primary-hover, active |
| `status` | success, warning, error, info, neutral |
| `spacing` | единая шкала 4px; не создавать произвольные расстояния |
| `radius` | control, card, dialog |
| `type` | heading, body, label, mono/metric |
| `motion` | краткие и отключаемые анимации |

Значения определяются в theme primitives; хардкод повторяющихся HEX, размеров и радиусов в feature-компонентах запрещён. Числа в метриках — tabular-nums, время и единицы явно обозначены.

## Базовые UI-элементы

`Button`, `IconButton`, `Input`, `Select`, `MultiSelect`, `Checkbox`, `Switch`, `Badge/Status`, `Tabs`, `Dialog`, `Drawer`, `Table`, `DataGrid`, `Tooltip`, `FormField`, `ErrorText`, `EmptyState`, `Skeleton`, `Toast`, `ConfirmDialog`, `Progress`, `MetricCard`, `Sparkline`, `Section`, `Toolbar`, `ActionMenu`.

Компоненты создаются только по реальной потребности. Отдельная дизайн-система не равна обязательному пакету каждого компонента.

## Компоновка

- Постоянный sidebar, структурированные группы, читаемые заголовки, стабильные места иконок.
- Плотные таблицы, но попадание курсором и управление с клавиатуры должны быть удобными.
- Сложные формы: разделы, responsive grid из 1/2 колонок, sticky footer с Save/Cancel, подсказка при несохранённых изменениях.
- Разрушительные действия визуально отделены и требуют подтверждения.
- «В сети» и «работает VPN» — разные статусы.
- Loading skeleton допустим только в ожидаемых областях; длительная операция показывает шаг и результат.

## Доступность

Иконки-кнопки имеют `aria-label`; видимый focus ring; клавиатурное управление Dialog/Tabs/Menu; контраст текста/состояний; `prefers-reduced-motion`; обработка экранов до 360px; не использовать только цвет для обозначения сбоя.

## Процесс изменения UI

Перед написанием новой страницы: проверить shared/ui и токены → выделить повторяемую композицию → описать состояния → реализовать → проверить desktop/mobile/keyboard/dark/light → добавить screenshot/E2E при необходимости.

## Экран схемы и таблицы

Для интерактивной карты VPN-нод допускается изолированный React-компонент с SVG (или Canvas **только при доказанной необходимости**). Использовать существующие `NodeCard`, `Status`, `Tooltip`, `ActionMenu`, `Drawer`, `DataTable`, цвета/токены статусов, типографику, поля, кнопки. Один UI-store содержит selection, фильтры и выбранный режим вида; графическая библиотека не должна приносить большой bundle без замеров. См. [topology-and-table.md](topology-and-table.md).

## Реализованные контракты UI — 9 ноября 2026

[Аудит, результаты и ограничения](ui-ux-audit-2026-11-09.md) — текущий источник по frontend. Sectioned editor сохраняет mounted sections и один baseline; invalid поле раскрывается, error связан с полем отдельно от стабильного accessible name. Dialog владеет focus/dirty/serial dismiss; workspace-variant фиксирует верхнюю границу и оставляет один основной scroll owner. Ошибки операций остаются в форме; Feedback предназначен для короткого подтверждения, один viewport внутри активного modal либо в оболочке, pause/dismiss/dedupe. Button pending скрывает исходную подпись визуально, сохраняя размеры. ErrorState различает 403 и Retry-After429; mutation unknown не называется гарантированным отказом. Новых библиотек/цветов нет.
