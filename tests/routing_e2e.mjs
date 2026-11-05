/** Real production frontend + Go API, no mocked responses and no VPN claims.
 * Requires Playwright (NODE_PATH is supported), a disposable initialized backend,
 * ARCVPN_E2E_URL and ARCVPN_E2E_PASSWORD_FILE. Never point at production.
 */
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
import {readFile, mkdir} from 'node:fs/promises';
import path from 'node:path';
const {chromium} = createRequire(import.meta.url)('playwright');
const origin = process.env.ARCVPN_E2E_URL;
const passwordFile = process.env.ARCVPN_E2E_PASSWORD_FILE;
assert(origin && passwordFile, 'Use a disposable backend and provide ARCVPN_E2E_URL / ARCVPN_E2E_PASSWORD_FILE');
const password = (await readFile(passwordFile, 'utf8')).trim();
const screenshots = path.resolve('tests/screenshots/routing');
await mkdir(screenshots, {recursive: true});
const browser = await chromium.launch({headless: true, ...(process.env.ARCVPN_CHROMIUM ? {executablePath: process.env.ARCVPN_CHROMIUM} : {})});
console.log(`Chromium ${browser.version()} / Playwright 1.58.2`);
const context = await browser.newContext({viewport: {width: 1440, height: 1000}, reducedMotion: 'reduce'});
const page = await context.newPage();
page.setDefaultTimeout(10000);
const errors = [];
page.on('pageerror', error => errors.push(error.message));
try {
  await page.goto(origin);
  await page.getByLabel('Логин', {exact: true}).fill('admin');
  await page.getByLabel('Пароль', {exact: true}).fill(password);
  await page.getByRole('button', {name: 'Войти', exact: true}).click();
  await page.getByRole('heading', {name: 'Обзор', exact: true}).waitFor();
  const sessionResponse = await context.request.get(`${origin}/api/session`);
  const session = await sessionResponse.json();
  const api = (method, url, data) => context.request.fetch(origin + url, {method, data, headers: {'X-CSRF-Token': session.csrf}});
  const registered = [];
  for (const name of ['Exit A', 'Exit B', 'Exit C']) {
    const enrollment = await api('POST', '/api/nodes/enroll', {name, address: '9.9.9.9', port: 443});
    assert.equal(enrollment.status(), 201);
    const grant = await enrollment.json();
    const registration = await context.request.post(`${origin}/api/nodes/register`, {data: {token: grant.token}});
    assert.equal(registration.status(), 201);
    registered.push({name, ...(await registration.json())});
  }
  const [a, b, c] = registered;
  await page.goto(`${origin}/routing`);
  await page.getByRole('button', {name: 'Серверы и каскады', exact: true}).click();
  await page.getByLabel('Название маршрута', {exact: true}).fill('Browser cascade');
  await page.locator(`.react-flow__node[data-id="${a.node_id}"]`).waitFor();
  // Exercise an actual pointer gesture, not a direct call to the model.
  const source = page.locator('.react-flow__node[data-id="local"] .react-flow__handle.source');
  const target = page.locator(`.react-flow__node[data-id="${a.node_id}"] .react-flow__handle.target`);
  const start = await source.boundingBox(); const end = await target.boundingBox();
  assert(start && end, 'connection handles missing');
  await page.mouse.move(start.x + start.width / 2, start.y + start.height / 2);
  await page.mouse.down();
  await page.mouse.move(end.x + end.width / 2, end.y + end.height / 2, {steps: 20});
  await page.mouse.up();
  await page.waitForFunction(id => document.querySelector('select[required]')?.value === id, a.node_id);
  assert.equal(await page.getByLabel('Режим', {exact: true}).inputValue(), 'cascade');
  await page.getByLabel('Новая стрелка', {exact: true}).selectOption('backup');
  const backupHandle = page.locator(`.react-flow__node[data-id="${b.node_id}"] .react-flow__handle.target`);
  await backupHandle.hover();
  await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
  const backupTarget = await backupHandle.boundingBox();
  const backupSource = await source.boundingBox();
  assert(backupTarget && backupSource);
  await page.mouse.move(backupSource.x + backupSource.width / 2, backupSource.y + backupSource.height / 2);
  await page.mouse.down();
  await page.mouse.move(backupTarget.x + backupTarget.width / 2, backupTarget.y + backupTarget.height / 2, {steps: 20});
  await page.mouse.up();
  await page.waitForFunction(() => [...document.querySelectorAll('.route-backup-options label')].some(label => label.textContent === 'Exit B' && label.querySelector('input').checked));
  await page.getByLabel('Exit C', {exact: true}).check();
  await page.getByRole('button', {name: 'Поднять резерв Exit C', exact: true}).click();
  await page.getByRole('button', {name: 'Проверить граф и изменения', exact: true}).click();
  await page.getByText('Граф проверен сервером', {exact: false}).waitFor();
  const saveResponse = page.waitForResponse(response => response.url() === `${origin}/api/routes` && response.request().method() === 'POST');
  await page.getByRole('button', {name: 'Сохранить черновик', exact: true}).click();
  assert.equal((await saveResponse).status(), 201);
  await page.getByText('Черновик сохранён. VPN и подписки не изменены.', {exact: true}).waitFor();
  let listing = await (await api('GET', '/api/routes')).json();
  let saved = listing.items.find(item => item.name === 'Browser cascade');
  assert(saved && !listing.publication_enabled);
  assert.deepEqual(saved.backup_exits, [c.node_id, b.node_id]);
  const blocked = await api('DELETE', `/api/nodes/${a.node_id}`);
  assert.equal(blocked.status(), 409);
  assert((await blocked.json()).error.includes('Browser cascade'));

  await page.getByRole('button', {name: 'Редактировать', exact: true}).click();
  await page.getByLabel('Основной выход', {exact: true}).selectOption(c.node_id);
  assert.deepEqual(await page.locator('.route-backup-order li>span').allTextContents(), ['Exit B']);
  await page.getByLabel('Входная нода', {exact: true}).selectOption(b.node_id);
  assert.equal(await page.locator('.route-backup-order li').count(), 0);
  await page.getByRole('button', {name: 'Сбросить форму', exact: true}).click();
  assert.equal(await page.getByLabel('Режим', {exact: true}).inputValue(), 'direct');
  assert.equal(await page.getByLabel('Входная нода', {exact: true}).inputValue(), 'local');

  // Opposite direction to an existing draft must be rejected including reserves.
  await page.getByLabel('Название маршрута', {exact: true}).fill('Cycle');
  await page.getByLabel('Режим', {exact: true}).selectOption('cascade');
  await page.getByLabel('Входная нода', {exact: true}).selectOption(a.node_id);
  await page.getByLabel('Основной выход', {exact: true}).selectOption('local');
  assert(await page.getByRole('button', {name: 'Сохранить черновик', exact: true}).isDisabled());
  await page.getByText('Связь создаёт цикл', {exact: false}).waitFor();
  await page.getByRole('button', {name: 'Сбросить форму', exact: true}).click();

  // A concurrent browser changes the persisted revision; keep our unsaved form.
  await page.getByRole('button', {name: 'Редактировать', exact: true}).click();
  const concurrent = await api('PUT', `/api/routes/${saved.id}`, {policy: {...saved, name: 'Concurrent version'}, expected_version: saved.version});
  assert.equal(concurrent.status(), 200);
  await page.getByLabel('Название маршрута', {exact: true}).fill('My unsaved changes');
  await page.getByRole('button', {name: 'Сохранить изменения', exact: true}).click();
  await page.getByRole('button', {name: 'Загрузить серверную версию (заменить правки)', exact: true}).waitFor();
  assert.equal(await page.getByLabel('Название маршрута', {exact: true}).inputValue(), 'My unsaved changes');
  await page.getByRole('button', {name: 'Загрузить серверную версию (заменить правки)', exact: true}).click();
  assert.equal(await page.getByLabel('Название маршрута', {exact: true}).inputValue(), 'Concurrent version');
  await page.getByRole('button', {name: 'Проверить граф и изменения', exact: true}).click();
  await page.getByText('Граф проверен сервером', {exact: false}).waitFor();
  await page.getByText('Параметры симуляции отказа', {exact: true}).click();
  await page.getByLabel('Ошибок до переключения', {exact: true}).fill('4');
  assert.equal(await page.locator('.route-preview').count(), 0, 'preview is stale after changing thresholds');

  // Export uses the real encrypted backup API and previews v4 route counts.
  const exported = await api('POST', '/api/backup/export', {password: 'browser-test-backup-password'});
  assert.equal(exported.status(), 200);
  const archive = await exported.json();
  const preview = await api('POST', '/api/backup/preview', {password: 'browser-test-backup-password', archive: archive.content});
  assert.equal(preview.status(), 200);
  const summary = await preview.json();
  assert.equal(summary.version, 8); assert.equal(summary.routes, 1); assert.equal(summary.routes_replaced, true);

  const sections = [['/', 'Обзор'], ['/users', 'Пользователи'], ['/vpn', 'Подключения'], ['/nodes', 'Серверы'], ['/routing', 'Маршруты'], ['/security', 'Безопасность'], ['/events', 'Журнал'], ['/backup', 'Импорт и экспорт']];
  for (const theme of ['dark', 'light']) {
    await context.addCookies([{name: 'arcvpn_theme', value: theme, url: origin}]);
    for (const width of [320, 390, 768, 1024, 1440]) {
      await page.setViewportSize({width, height: 1000});
      for (const [route, heading] of sections) {
        await page.goto(origin + route, {waitUntil: 'networkidle'});
        await page.getByRole('heading', {name: heading, exact: true}).first().waitFor();
        assert.equal(await page.evaluate(() => document.documentElement.dataset.theme), theme);
        assert((await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)) <= 1, `overflow ${theme}/${width}/${route}`);
      }
      await page.goto(`${origin}/routing`, {waitUntil: 'networkidle'});
      await page.getByRole('button', {name: 'Серверы и каскады', exact: true}).click();
      if (width <= 680) assert.equal(await page.getByRole('button', {name: 'Таблица', exact: true}).getAttribute('aria-pressed'), 'true');
      await page.getByRole('button', {name: 'Редактировать', exact: true}).click();
      await page.evaluate(() => window.scrollTo(0, 0));
      await page.screenshot({path: path.join(screenshots, `${theme}-${width}-routing.png`), fullPage: true});
      await page.getByRole('button', {name: 'Схема', exact: true}).click();
      await page.locator('.react-flow__edge-path').first().waitFor({state: 'attached'});
      assert((await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)) <= 1, `canvas overflow ${width}`);
      await page.evaluate(() => window.scrollTo(0, 0));
      await page.screenshot({path: path.join(screenshots, `${theme}-${width}-canvas.png`), fullPage: true});
    }
  }
  assert.deepEqual(errors, [], 'JavaScript errors');
  listing = await (await api('GET', '/api/routes')).json(); saved = listing.items.find(item => item.id === saved.id);
  await page.goto(`${origin}/routing`);
  await page.getByRole('button', {name: 'Серверы и каскады', exact: true}).click();
  page.once('dialog', dialog => dialog.accept());
  await page.getByRole('button', {name: 'Удалить черновик', exact: true}).click();
  await page.getByText('Черновик удалён.', {exact: true}).waitFor();
  assert.equal((await api('DELETE', `/api/nodes/${a.node_id}`)).status(), 200);
  console.log('PASS: real React/Go API; pointer connection; backup ordering; form cleanup; cycles; CAS conflict; node dependency; encrypted v6 preview; 8 pages × 5 widths × 2 themes; no JS errors. VPN NOT TESTED.');
} catch (error) {
  await page.screenshot({path: path.join(screenshots, "failure.png"), fullPage: true}).catch(() => {});
  console.error("Browser errors:", errors);
  throw error;
} finally {
  await context.close(); await browser.close();
}
