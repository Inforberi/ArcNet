import {test} from 'node:test';
import assert from 'node:assert/strict';
import {serverOptionChanges,clientOptionChanges,connectionAddress,connectionService,connectionTransport,connectionProtection} from '../src/features/connections/presentation.ts';
test('server diff preserves legacy defaults and makes restart changes explicit',()=>{
 assert.deepEqual(serverOptionChanges('vless-reality',{}, {sniffing:true}),[]);
 assert.deepEqual(serverOptionChanges('vless-xhttp',{}, {sniffing:true}),['Sniffing: выключен → включён']);
 assert.deepEqual(serverOptionChanges('hysteria2',{}, {udp_idle_seconds:60,quic_idle_seconds:30}),[]);
 assert.deepEqual(serverOptionChanges('hysteria2',{}, {udp_idle_seconds:90}),['UDP timeout: 60 → 90']);
 assert.deepEqual(serverOptionChanges('vless-reality',{}, {flow:'none'}),['Flow: Vision → без flow']);
 assert.deepEqual(serverOptionChanges('hysteria2',{}, {salamander:true,up_mbps:10}),['Salamander: выключен → включён','От сервера клиенту, Мбит/с: 0 → 10']);
});
test('client diff uses actual defaults and preserves disabled keepalive',()=>{
 assert.deepEqual(clientOptionChanges('wireguard',{}, {mtu:1280,dns:'1.1.1.1',keepalive:25}),[]);
 assert.deepEqual(clientOptionChanges('wireguard',{}, {keepalive:0}),['Keepalive: 25 → 0']);
 assert.deepEqual(clientOptionChanges('vless-xhttp',{}, {xhttp_mode:'packet-up'}),['Режим XHTTP: auto → packet-up']);
 assert.deepEqual(clientOptionChanges('hysteria2',{},{}),[]);
});

test('XHTTP stays distinct from TCP and endpoints support IPv6',()=>{
 assert.equal(connectionTransport('vless-xhttp'),'XHTTP');
 assert.equal(connectionTransport('vless-reality'),'TCP / RAW');
 assert.equal(connectionProtection('vless-xhttp'),'REALITY');
 assert.equal(connectionAddress('2001:db8::1',3443),'[2001:db8::1]:3443');
 assert.equal(connectionAddress('vpn.example.com',443),'vpn.example.com:443');
});
test('unknown health never appears as running or failed',()=>{
 for(const state of [undefined,'unknown','unverified','unexpected'])assert.equal(connectionService(state).variant,'neutral');
 assert.equal(connectionService('active').variant,'green');
 assert.equal(connectionService('failed').variant,'red');
 assert.equal(connectionService('inactive').label,'Не запущена');
});
