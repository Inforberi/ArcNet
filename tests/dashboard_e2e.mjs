/** Real Go/React dashboard with synthetic authenticated node reports, not VPS/VPN proof. */
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
import {mkdir, readFile} from 'node:fs/promises';
const {chromium} = createRequire(import.meta.url)('playwright');
const origin = process.env.ARCVPN_E2E_URL;
assert(origin && process.env.ARCVPN_E2E_PASSWORD_FILE, 'Disposable test backend required');
const password = (await readFile(process.env.ARCVPN_E2E_PASSWORD_FILE, 'utf8')).trim();
const browser = await chromium.launch({headless: true, ...(process.env.ARCVPN_CHROMIUM ? {executablePath: process.env.ARCVPN_CHROMIUM} : {})});
const context = await browser.newContext({viewport: {width: 1440, height: 1000}});
const page = await context.newPage(); page.setDefaultTimeout(15000);
const errors = []; page.on('pageerror', error => errors.push(error.message));
const registered = [];
await mkdir('tests/screenshots/dashboard', {recursive: true});
try {
 await page.goto(origin);
 await page.getByLabel('Логин', {exact: true}).fill('admin'); await page.getByLabel('Пароль', {exact: true}).fill(password);
 await page.getByRole('button', {name: 'Войти', exact: true}).click(); await page.getByRole('heading', {name: 'Обзор', exact: true}).waitFor();
 const session = await (await context.request.get(origin + '/api/session')).json();
 const api = (method, path, data) => context.request.fetch(origin + path, {method, data, headers: {'X-CSRF-Token': session.csrf}});
 for (const name of ['Dashboard main server with a long readable name', 'Dashboard offline', 'Dashboard legacy', ...Array.from({length: 20}, (_, index) => `Dashboard extra ${index}`)]) {
  const enrollment = await api('POST', '/api/nodes/enroll', {name, address: '9.9.9.9', port: 443}); assert.equal(enrollment.status(), 201);
  const grant = await enrollment.json(); const reply = await context.request.post(origin + '/api/nodes/register', {data: {token: grant.token}}); assert.equal(reply.status(), 201);
  registered.push({name, ...await reply.json()});
 }
 const [primary, offline, legacy] = registered;
 const heartbeat = node => context.request.post(origin + '/api/nodes/heartbeat', {headers: {Authorization: `Bearer ${node.secret}`}, data: {version: 'dashboard-fixture', protocol_version: 1, xray_state: 'active', memory_total: 1073741824, memory_available: 536870912, load_1: 0.4, uptime_seconds: 1000}});
 assert.equal((await heartbeat(primary)).status(), 204); assert.equal((await heartbeat(legacy)).status(), 204);
 const startedAt = Math.floor(Date.now() / 1000);
 const report = () => ({version: 1, node_id: primary.node_id, snapshot: {sampled_at: Math.floor(Date.now() / 1000), epoch: 'fixture-node-boot', network_interfaces: ['eth0'], cpus: 2, cpu_percent: 42, memory_total: 1073741824, memory_available: 536870912, swap_total: 0, swap_free: 0, disk_total: 10737418240, disk_free: 5368709120, load_1: 0.4, uptime_seconds: 1000, rx_bytes: 1000000 + 1024 * (Math.floor(Date.now() / 1000) - startedAt), tx_bytes: 2000000 + 2048 * (Math.floor(Date.now() / 1000) - startedAt), rx_per_second: 1024, tx_per_second: 2048, tcp_sockets: 12, udp_sockets: 3}});
 const send = () => context.request.post(origin + '/api/nodes/telemetry', {headers: {Authorization: `Bearer ${primary.secret}`}, data: report()});
 assert.equal((await send()).status(), 204);
 const inspect = await api('GET', `/api/servers/${primary.node_id}/telemetry`);
 assert.equal(inspect.headers()['cache-control'], 'no-store'); const state = await inspect.json(); assert.equal(state.server.vpn, 'unknown'); assert.equal(state.vpn_traffic_verified, false);
 await page.reload({waitUntil: 'networkidle'});
 await page.getByLabel('Сервер на дашборде', {exact: true}).selectOption(primary.node_id);
 await page.locator('.metrics .metric .value').first().filter({hasText: '42%'}).waitFor();
 assert.equal(await page.getByRole('img', {name: /История скорости сети ОС/}).count(), 0, 'history invented before a second minute');
 await page.getByLabel('Сервер на дашборде', {exact: true}).selectOption(legacy.node_id);
 await page.getByText('Агент передаёт базовые показатели.', {exact: false}).waitFor();
 assert.equal(await page.locator('.metrics .metric .value').first().innerText(), 'Нет данных');
 await page.getByLabel('Сервер на дашборде', {exact: true}).selectOption(offline.node_id);
 await page.getByText('Нет свежих показателей', {exact: true}).first().waitFor();
 assert.equal(await page.locator('.fleet-server').count(), 20);
 await page.getByRole('button', {name: /Показать ещё/}).click(); assert.equal(await page.locator('.fleet-server').count(), 24);
 await page.getByLabel('Найти сервер', {exact: true}).fill('legacy'); assert.equal(await page.locator('.fleet-server').count(), 1);
 await page.getByLabel('Найти сервер', {exact: true}).fill('');
 // Wait for a real controller minute boundary. No database fixture injection,
 // backdated receipt timestamps, intercepted APIs or generated fake history.
 await new Promise(resolve => setTimeout(resolve, 11000));
 assert.equal((await send()).status(), 204);
 const wait = Math.max(11000, (61 - new Date().getSeconds()) * 1000);
 console.log(`Dashboard functional checks passed; waiting ${Math.round(wait / 1000)}s for the second real history bucket.`);
 await new Promise(resolve => setTimeout(resolve, wait));
 assert.equal((await heartbeat(primary)).status(), 204); assert.equal((await send()).status(), 204);
 for (const theme of ['light', 'dark']) {
  await context.addCookies([{name: 'arcvpn_theme', value: theme, url: origin}]);
  for (const width of [320, 390, 768, 1024, 1440]) {
   await page.setViewportSize({width, height: 1000}); await page.goto(origin, {waitUntil: 'networkidle'});
   await page.getByLabel('Сервер на дашборде', {exact: true}).selectOption(primary.node_id);
   const graph = page.getByRole('img', {name: /История скорости сети ОС/}); await graph.waitFor();
   await page.getByLabel('Найти сервер', {exact: true}).fill('Dashboard main');
   assert.equal(await page.evaluate(() => document.documentElement.dataset.theme), theme);
   assert((await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)) <= 1, `dashboard overflow ${theme}/${width}`);
   await graph.focus(); await graph.press('ArrowLeft');
   await page.screenshot({path: `tests/screenshots/dashboard/${theme}-${width}.png`, fullPage: true});
  }
 }
 assert.deepEqual(errors, []);
 console.log('PASS: actual dashboard UI/API; synthetic node reports/legacy/offline; real minute history; 5 widths × 2 themes; no JS errors. VPS/VPN NOT TESTED.');
} catch (error) {await page.screenshot({path: 'tests/screenshots/dashboard/failure.png', fullPage: true}).catch(() => {}); throw error;}
finally {
 const session = await (await context.request.get(origin + '/api/session')).json().catch(() => ({}));
 for (const node of registered) await context.request.delete(origin + `/api/nodes/${node.node_id}`, {headers: {'X-CSRF-Token': session.csrf ?? ''}}).catch(() => {});
 await context.close(); await browser.close();
}
