import {test} from 'node:test';
import assert from 'node:assert/strict';
import {presenceState,presenceText,type UserTraffic} from '../src/features/users/traffic.ts';

const item=(sources:NonNullable<UserTraffic['presence']>['sources'])=>({presence:{state:'online',observed_at:100,complete_coverage:false,sources}} as UserTraffic);
test('presence expires each source separately instead of trusting the freshest unrelated observation',()=>{
 const value=item([{source:'xray',state:'online',observed_at:80},{source:'wireguard',state:'unknown',observed_at:100}]);
 assert.equal(presenceState(value,false,100),'online');assert.equal(presenceState(value,false,101),'stale');
 assert.equal(presenceText(undefined),'Нет данных');assert.equal(presenceText(value,true,100),'Данные устарели');
});
test('only complete confirmed absence is offline; WG configured peers remain unknown',()=>{
 assert.equal(presenceState(item([{source:'xray',state:'offline',observed_at:100}]),false,100),'offline');
 assert.equal(presenceState(item([{source:'xray',state:'offline',observed_at:100},{source:'wireguard',state:'unknown',observed_at:100}]),false,100),'unknown');
 assert.equal(presenceState(item([{source:'wireguard',state:'recent',observed_at:100,last_handshake_at:90}]),false,100),'recent');
});
