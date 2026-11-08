import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
const read=(path:string)=>readFileSync(new URL(path,import.meta.url),'utf8');
test('v6 themes define every color role used by shared component styles',()=>{
 const tokens=read('../src/shared/design-system/tokens.css');
 const declared=new Set([...tokens.matchAll(/(--[\w-]+)\s*:/g)].map(match=>match[1]));
 for(const path of ['../src/style.css','../src/shared/design-system/components.css']){
  const css=read(path);
  assert.equal(/#[\da-f]{3,8}\b/i.test(css),false,`${path} must consume color tokens`);
  for(const match of css.matchAll(/var\((--[\w-]+)/g))assert.ok(declared.has(match[1]),`undefined token ${match[1]}`);
 }
 const dark=tokens.split('/* A theme')[0];const light=tokens.split(':root[data-theme="light"]')[1];
 for(const name of ['--bg','--surface','--stroke','--accent','--focus','--selected','--text','--chart-received','--chart-sent']){
  assert.ok(dark.includes(name+':'),`dark missing ${name}`);assert.ok(light.includes(name+':'),`light missing ${name}`);
 }
});

test('device screen does not create manual identities or silently replace a shared key',()=>{
 const source=read('../src/features/users/DevicesDialog.tsx');
 assert.equal(source.includes('Добавить устройство'),false);
 assert.equal(source.includes("change('POST'"),false);
 assert.ok(source.includes('DELETE'));
 assert.ok(source.includes('Исполнение ограничения устройств или IP ещё не подключено'));
});
