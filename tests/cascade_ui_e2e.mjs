// Real production React + disposable Go/SQLite API. Agent ACKs are fixtures,
// never Xray/systemd/VPS/egress acceptance. No credentials are printed.
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
import {readFile, mkdir} from 'node:fs/promises';
const {chromium} = createRequire(import.meta.url)('playwright');
const origin = process.env.ARCVPN_E2E_URL ?? 'http://127.0.0.1:38567';
const fixture = JSON.parse(await readFile(process.env.ARCVPN_CASCADE_FIXTURE_FILE, 'utf8'));
const shots = process.env.ARCVPN_CASCADE_SCREENSHOTS;
if (shots) await mkdir(shots, {recursive: true});
const browser = await chromium.launch({headless: true, ...(process.env.ARCVPN_CHROMIUM ? {executablePath: process.env.ARCVPN_CHROMIUM} : {})});
const context = await browser.newContext({viewport: {width: 390, height: 844}, reducedMotion: 'reduce'});
await context.addCookies([{name: 'arcvpn_session', value: fixture.session, url: origin, httpOnly: true, sameSite: 'Strict'}]);
const page = await context.newPage();
page.setDefaultTimeout(10000);
const errors = [];
page.on('pageerror', e => errors.push(e.message));
const request = async (method, path, data) => {
  const result = await context.request.fetch(origin + path, {method, data, headers: {'X-CSRF-Token': fixture.csrf}});
  assert(result.ok(), `API ${method} ${path}: ${result.status()}`);
  return result.status() === 204 ? null : result.json();
};
const ack = async (index) => {
  const node = fixture.nodes[index];
  const command = await context.request.get(`${origin}/api/nodes/poll?node_id=${encodeURIComponent(node.id)}`, {headers: {Authorization: `Bearer ${node.secret}`}});
  assert.equal(command.status(), 200, 'expected staged node command');
  const value = await command.json();
  const response = await context.request.post(origin + '/api/nodes/ack', {headers: {Authorization: `Bearer ${node.secret}`}, data: {node_id: node.id, operation_id: value.operation_id, revision: value.revision, sha256: value.sha256, result: 'applied'}});
  assert.equal(response.status(), 204);
};
try {
  await page.goto(origin + '/routing');
  await page.getByRole('button', {name: 'Серверы и каскады', exact: true}).click();
  await page.getByRole('button', {name: 'Применить каскад', exact: true}).click();
  const dialog = page.getByRole('dialog', {name: 'Применение: Transparent cascade'});
  await dialog.getByRole('button', {name: 'Предпросмотр изменений'}).click();
  await dialog.getByText(/Все 1 текущих доступов и их ключи сохраняются/).waitFor();
  const apply = dialog.getByRole('button', {name: 'Применить настройки', exact: true});
  assert(await apply.isDisabled(), 'apply needs explicit confirmation');
  // Form changes invalidate the reviewed plan and preserve manual input.
  await dialog.getByRole('spinbutton', {name: /TCP-порт межсерверного канала на B/}).fill('15444');
  assert.equal(await apply.count(), 0, 'port change must invalidate preview');
  await dialog.getByRole('button', {name: 'Предпросмотр изменений'}).click();
  await dialog.getByText('Что изменится', {exact: true}).waitFor();
  for (const width of [390, 1280]) {
    await page.setViewportSize({width, height: 900});
    assert((await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)) <= 1, `page overflow ${width}`);
    assert(await dialog.evaluate(el => el.scrollWidth - el.clientWidth <= 1), `dialog overflow ${width}`);
    if (shots) await page.screenshot({path: `${shots}/preview-${width}.png`, fullPage: true});
  }
  await page.setViewportSize({width: 390, height: 844});
  await dialog.getByLabel('Подтверждаю смену выхода на B для этого подключения', {exact: true}).check();
  await apply.click();
  await dialog.waitFor({state: 'hidden'});
  await page.getByText('Настройка выхода B', {exact: true}).waitFor();
  await ack(1);
  await page.getByText('Настройка входа A', {exact: true}).waitFor();
  await ack(0);
  await page.getByText('Применено · VPN не проверен', {exact: true}).waitFor();
  assert.equal(await page.getByText('Клиенты продолжают использовать прежний профиль.', {exact: false}).count(), 1);
  await page.reload();
  await page.getByRole('button', {name: 'Серверы и каскады', exact: true}).click();
  await page.getByText('Применено · VPN не проверен', {exact: true}).waitFor();
  assert((await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)) <= 1, 'applied status mobile overflow');
  if (shots) await page.screenshot({path: `${shots}/applied-390.png`, fullPage: true});
  page.on('dialog', dialog => void dialog.accept());
  await page.getByRole('button', {name: 'Откатить каскад', exact: true}).click();
  await page.getByText('Восстановление входа A', {exact: true}).waitFor();
  await ack(0); await ack(1);
  await page.getByText('Прежние настройки восстановлены', {exact: true}).waitFor();
  await page.getByRole('button', {name: 'Удалить завершённый журнал', exact: true}).click();
  await page.getByRole('button', {name: 'Применить каскад', exact: true}).waitFor();
  const result = await request('GET', '/api/cascade-deployments');
  assert.equal(result.items.length, 0);
  assert.deepEqual(errors, []);
  console.log('PASS: real UI/API preview, input invalidation, explicit apply, B/A status, reload, rollback and cleanup; 390/1280 layout; synthetic ACKs, VPN NOT TESTED.');
} catch (error) {
  if (shots) await page.screenshot({path: `${shots}/failure.png`, fullPage: true});
  throw error;
} finally {
  await context.request.post(origin + '/__fixture_done').catch(() => {});
  await browser.close();
}
