// Real disposable HTTP/SQLite/UI, with simulated engines. No VPN claim.
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
import {mkdir} from 'node:fs/promises';
const {chromium} = createRequire(import.meta.url)('playwright');
const origin = process.env.ARCVPN_E2E_URL;
assert(origin && new URL(origin).hostname === '127.0.0.1', 'Disposable loopback fixture required');
const browser = await chromium.launch({headless:true, ...(process.env.ARCVPN_CHROMIUM ? {executablePath:process.env.ARCVPN_CHROMIUM} : {})});
const context = await browser.newContext({viewport:{width:390,height:844}});
await context.addCookies([{name:'arcvpn_session',value:'private-fixture-session',url:origin,httpOnly:true,sameSite:'Strict'}]);
const page = await context.newPage(); page.setDefaultTimeout(12000);
const errors = []; page.on('pageerror', error => errors.push(error.message));
const api = (method, path, data) => context.request.fetch(origin+path,{method,data,headers:{'X-CSRF-Token':'fixture-csrf'}});
await mkdir('tests/screenshots/first-protocol',{recursive:true});
try {
 await page.goto(origin+'/vpn',{waitUntil:'domcontentloaded'});
 assert.equal((await (await api('GET','/api/vpn')).json()).configured,false);
 const form = page.locator('form').filter({has:page.getByLabel('Тип подключения',{exact:true})});
 await form.getByLabel('Тип подключения',{exact:true}).selectOption('hysteria2');
 await form.getByRole('button',{name:'Создать и применить',exact:true}).click();
 await page.getByText('Hysteria2: конфигурация применена.',{exact:false}).waitFor();
 const vpn = await (await api('GET','/api/vpn')).json();
 assert.equal(vpn.configured,false); assert.equal(vpn.address,'198.51.100.25');
 await form.getByLabel('Тип подключения',{exact:true}).selectOption('wireguard');
 await form.getByRole('button',{name:'Создать и применить',exact:true}).click();
 await page.getByText('WireGuard: конфигурация применена.',{exact:false}).waitFor();
 assert.equal((await api('POST','/api/users',{name:'First protocol user'})).status(),201);
 const connections = await (await api('GET','/api/users/1/connections')).json();
 assert.deepEqual(connections.items.map(p=>p.kind).sort(),['hysteria2','wireguard']);
 const subscription = await (await api('GET','/api/users/1/subscription')).json();
 assert.equal(subscription.links.length,1); assert(subscription.link.startsWith('hysteria2://'));
 await page.goto(origin+'/users',{waitUntil:'domcontentloaded'});
 await page.getByRole('button',{name:'Подключить',exact:true}).click();
 await page.getByLabel('Прямое подключение',{exact:true}).waitFor();
 assert.equal(await page.getByRole('button',{name:'Копировать прямую ссылку',exact:true}).count(),1);
 assert.equal(await page.getByRole('button',{name:'QR VLESS',exact:true}).count(),0);
 assert.equal((await api('POST','/api/users/1/devices',{name:'Phone',expected_version:0})).status(),200);
 const device = (await (await api('GET','/api/users/1/devices')).json()).items[0];
 const links = await (await api('GET',`/api/users/1/devices/${device.id}/links`)).json();
 assert.equal(links.links.length,1); assert(links.links[0].startsWith('hysteria2://')); assert(links.wireguard_url);
 const download = await context.request.get(origin+new URL(links.wireguard_url).pathname);
 assert.equal(download.status(),200); assert((await download.text()).includes('[Interface]'));
 for (const width of [390,1440]) {
  await page.setViewportSize({width,height:900}); await page.goto(origin+'/vpn',{waitUntil:'domcontentloaded'});
  await page.getByRole('heading',{name:'Подключения',exact:true}).waitFor();
  await page.locator('.protocol-card').filter({hasText:'WireGuard'}).waitFor();
  assert((await page.evaluate(()=>document.documentElement.scrollWidth-document.documentElement.clientWidth))<=1);
  await page.screenshot({path:`tests/screenshots/first-protocol/${width}.png`,fullPage:true});
 }
 assert.deepEqual(errors,[]);
 console.log('PASS: Hysteria2 first, WireGuard next, no implicit Reality, user/device export, mobile/desktop. Engines simulated; VPN NOT TESTED.');
} finally {
 await context.request.post(origin+'/fixture-done').catch(()=>{});
 await browser.close();
}
