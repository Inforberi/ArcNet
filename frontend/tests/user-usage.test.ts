import assert from 'node:assert/strict';
import test from 'node:test';
import {quotaBytes, usageGB} from '../src/features/users/usage.ts';
test('packages use decimal GB without accidental unlimited or invalid quota', () => {
 assert.equal(quotaBytes('1'), 1_000_000_000);
 assert.equal(quotaBytes('0,125'), 125_000_000);
 for (const value of ['0','-1','','NaN','Infinity','1e3','0.0001','9000001']) assert.equal(quotaBytes(value),undefined,value);
 assert.match(usageGB(1_000_000_000),/1 ГБ/);
});

test('access labels distinguish policy, missing accounting and denied connections',async()=>{
 const {userAccess}=await import('../src/features/users/usage.ts');
 const user={enabled:true,preferences:{allowed_kinds:null}};
 const pack={block_reason:'',quota_bytes:0};
 assert.equal(userAccess(user as never).label,'Нет данных');
 assert.equal(userAccess(user as never,pack as never).allowed,true);
 assert.equal(userAccess({...user,enabled:false} as never,pack as never).allowed,false);
 assert.equal(userAccess({...user,preferences:{allowed_kinds:[]}} as never,pack as never).label,'Нет подключений');
 assert.equal(userAccess(user as never,{...pack,block_reason:'expired'} as never).label,'Срок истёк');
});
test('small traffic amounts never round to zero GB',async()=>{
 const {usageAmount}=await import('../src/features/users/usage.ts');
 assert.equal(usageAmount(0),'0 Б');assert.equal(usageAmount(243),'243 Б');assert.equal(usageAmount(1000),'1 КБ');assert.equal(usageAmount(1000000),'1 МБ');assert.equal(usageAmount(1000000000),'1 ГБ');
});
