/** Disposable real Go/React fixture only. No intercepted API or VPN traffic. */
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
import {readFile, mkdir} from 'node:fs/promises';
const {chromium} = createRequire(import.meta.url)('playwright');
const origin = process.env.ARCVPN_E2E_URL;
assert(origin && process.env.ARCVPN_E2E_PASSWORD_FILE, 'Disposable backend credentials required');
const password = (await readFile(process.env.ARCVPN_E2E_PASSWORD_FILE, 'utf8')).trim();
const browser = await chromium.launch({headless: true, ...(process.env.ARCVPN_CHROMIUM ? {executablePath: process.env.ARCVPN_CHROMIUM} : {})});
const context = await browser.newContext({viewport: {width: 390, height: 844}, reducedMotion: 'reduce'});
const page = await context.newPage(); page.setDefaultTimeout(10000);
const errors = []; page.on('pageerror', error => errors.push(error.message));
await mkdir('tests/screenshots/traffic-profiles', {recursive: true});
try {
 await page.goto(origin);
 await page.getByLabel('Логин', {exact: true}).fill('admin');
 await page.getByLabel('Пароль', {exact: true}).fill(password);
 await page.getByRole('button', {name: 'Войти', exact: true}).click();
 await page.getByRole('heading', {name: 'Обзор', exact: true}).waitFor();
 const session = await (await context.request.get(origin + '/api/session')).json();
 const api = (method, path, data) => context.request.fetch(origin + path, {method, data, headers: {'X-CSRF-Token': session.csrf}});
 await page.goto(origin + '/routing');
 assert.equal(await page.locator('.react-flow').count(), 0, 'heavy graph should load separately');
 await page.getByRole('button', {name: 'Создать профиль', exact: true}).click();
 const dialog = page.getByRole('dialog');
 await page.getByLabel('Название профиля', {exact: true}).fill('Browser traffic rules');
 await page.getByRole('checkbox', {name: /Использовать правила маршрутизации/}).check();
 await page.getByLabel('Напрямую — домены, IP и Geo-категории', {exact: true}).fill('example.com');
 await page.getByLabel('Импорт списка адресов', {exact: true}).setInputFiles({name: 'domains.txt', mimeType: 'text/plain', buffer: Buffer.from('# list\nexample.com\n192.0.2.4/24')});
 await page.getByText('В форму добавлено 1 записей.', {exact: false}).waitFor();
 await page.getByText('Проверить адрес', {exact: true}).click();
 await page.getByLabel('Домен или IP для проверки', {exact: true}).fill('sub.example.com');
 await page.getByRole('button', {name: 'Проверить правило', exact: true}).click();
 await dialog.getByText('Совпало правило профиля', {exact: false}).waitFor();
 await page.getByRole('button', {name: 'Сохранить профиль', exact: true}).click();
 await dialog.waitFor({state: 'detached'});
 let listing = await (await api('GET', '/api/routing-profiles')).json();
 let saved = listing.items.find(item => item.name === 'Browser traffic rules');
 assert(saved && saved.enabled && !listing.server_application_enabled);
 assert.deepEqual(saved.direct, ['domain:example.com', '192.0.2.0/24']);
 const exported = await (await api('GET', `/api/routing-profiles/${saved.id}/export`)).json();
 assert.equal(exported.format, 'arcvpn-routing-profile');
 // Switching off preserves rules and restores the base model action.
 await page.getByRole('button', {name: 'Настроить', exact: true}).click();
 await page.getByRole('checkbox', {name: /Использовать правила маршрутизации/}).uncheck();
 await page.getByRole('button', {name: 'Сохранить профиль', exact: true}).click();
 await dialog.waitFor({state: 'detached'});
 listing = await (await api('GET', '/api/routing-profiles')).json(); saved = listing.items.find(item => item.id === saved.id);
 assert(!saved.enabled); assert.equal(saved.direct.length, 2);
 const decision = await (await api('POST', '/api/routing-profiles/check', {profile: saved, address: 'example.com'})).json();
 assert.equal(decision.action, 'proxy'); assert.equal(decision.network_verified, false);
 // Closing a draft retains input; CAS protects concurrent edits.
 await page.getByRole('button', {name: 'Настроить', exact: true}).click();
 await page.getByLabel('Название профиля', {exact: true}).fill('Unsaved local name');
 await page.getByRole('button', {name: 'Закрыть', exact: true}).click();
 await page.getByRole('button', {name: 'Продолжить правки', exact: true}).click();
 assert.equal(await page.getByLabel('Название профиля', {exact: true}).inputValue(), 'Unsaved local name');
 const updated = await api('PUT', `/api/routing-profiles/${saved.id}`, {profile: {...saved, name: 'Server newer name'}, expected_version: saved.version});
 assert.equal(updated.status(), 200);
 await page.getByRole('button', {name: 'Сохранить профиль', exact: true}).click();
 await page.getByRole('button', {name: 'Загрузить серверную версию', exact: true}).waitFor();
 assert.equal(await page.getByLabel('Название профиля', {exact: true}).inputValue(), 'Unsaved local name');
 await page.getByRole('button', {name: 'Загрузить серверную версию', exact: true}).click();
 assert.equal(await page.getByLabel('Название профиля', {exact: true}).inputValue(), 'Server newer name');
 for (const theme of ['light', 'dark']) {
  await context.addCookies([{name: 'arcvpn_theme', value: theme, url: origin}]);
  for (const width of [320, 390, 768, 1024, 1440]) {
   await page.setViewportSize({width, height: 844});
   await page.reload({waitUntil: 'networkidle'});
   await page.getByRole('button', {name: 'Настроить', exact: true}).click();
   await page.getByRole('checkbox', {name: /Использовать правила маршрутизации/}).check();
   const box = await dialog.boundingBox(); assert(box.width <= width + 1 && box.x >= -1, `modal width ${width}`);
   assert(await dialog.evaluate(node => node.scrollWidth <= node.clientWidth + 1), `modal overflow ${width}`);
   await page.getByRole('button', {name: 'Сохранить профиль', exact: true}).scrollIntoViewIfNeeded();
   await page.screenshot({path: `tests/screenshots/traffic-profiles/${theme}-${width}.png`, fullPage: true});
   await page.getByRole('button', {name: 'Закрыть', exact: true}).click();
  }
 }
 // Import is a preview; metadata is reset and nothing is silently persisted.
 await page.reload();
 await page.getByLabel('Импорт профиля маршрутизации', {exact: true}).setInputFiles({name: 'profile.json', mimeType: 'application/json', buffer: Buffer.from(JSON.stringify(exported))});
 await page.getByRole('heading', {name: 'Новый профиль маршрутизации', exact: true}).waitFor();
 assert.equal((await (await api('GET', '/api/routing-profiles')).json()).items.length, 1);
 await page.getByRole('button', {name: 'Закрыть', exact: true}).click();
 // SNI catalogue validation uses the actual API without connecting externally.
 await page.goto(origin + '/vpn');
 await page.getByText('Свой список SNI', {exact: true}).click();
 await page.getByLabel('Кандидаты, по одному на строке', {exact: true}).fill('metadata.internal');
 await page.getByRole('button', {name: 'Заменить каталог кандидатов', exact: true}).click();
 await page.getByText('target и SNI должны быть корректными публичными DNS-именами', {exact: true}).waitFor();
 assert.equal((await (await api('GET', '/api/targets/catalog')).json()).catalog.version, 1);
 listing = await (await api('GET', '/api/routing-profiles')).json(); saved = listing.items[0];
 assert.equal((await api('DELETE', `/api/routing-profiles/${saved.id}`, {expected_version: saved.version})).status(), 200);
 assert.deepEqual(errors, []);
 console.log('PASS traffic profiles: actual CRUD/import/export/list preview/off/CAS/draft retention, 5 widths × 2 themes, SNI SSRF rejection; no real VPN or external target probe.');
} catch (error) {
 await page.screenshot({path: 'tests/screenshots/traffic-profiles/failure.png', fullPage: true}).catch(() => {}); throw error;
} finally {await context.close(); await browser.close();}
