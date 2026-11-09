import {test} from 'node:test';
import assert from 'node:assert/strict';
import {api,ApiError,retryQuery,retryAfter} from '../src/shared/api.ts';

test('Retry-After supports seconds and HTTP dates without unbounded timers',()=>{
 const now=Date.parse('2026-11-09T10:00:00Z');
 assert.equal(retryAfter('12',now),now+12000);
 assert.equal(retryAfter('Mon, 09 Nov 2026 10:01:00 GMT',now),now+60000);
 assert.equal(retryAfter('garbage',now),0);
 assert.equal(retryAfter('-10',now),0);
 assert.equal(retryAfter('999999999999',now),now+86400000);
});

test('query retry never repeats authorization, conflict or throttled requests',()=>{
 for(const status of [400,401,403,409,422,429])assert.equal(retryQuery(0,new ApiError(status,'test')),false);
 assert.equal(retryQuery(0,new ApiError(503,'test')),true);
 assert.equal(retryQuery(1,new Error('network')),false);
});

test('lost mutation response is explicitly unknown; GET remains a recoverable load error',async()=>{
 const original=globalThis.fetch;
 globalThis.fetch=async()=>{throw new TypeError('connection lost')};
 try{
  await assert.rejects(api('/fixture','PUT',{}),(error:unknown)=>error instanceof ApiError&&error.outcomeUnknown&&error.message.includes('Результат операции неизвестен'));
  await assert.rejects(api('/fixture'),(error:unknown)=>error instanceof ApiError&&!error.outcomeUnknown&&error.message.includes('повторите загрузку'));
 }finally{globalThis.fetch=original}
});

test('invalid mutation response and server error never become confirmed success',async()=>{
 const original=globalThis.fetch;
 try{
  globalThis.fetch=async()=>new Response('not JSON',{status:200});
  await assert.rejects(api('/fixture','POST',{}),(error:unknown)=>error instanceof ApiError&&error.outcomeUnknown);
  globalThis.fetch=async()=>new Response('{"error":"Apply interrupted"}',{status:503});
  await assert.rejects(api('/fixture','PUT',{}),(error:unknown)=>error instanceof ApiError&&error.outcomeUnknown&&error.message.includes('Apply interrupted'));
  globalThis.fetch=async()=>new Response('{"error":"Revision conflict"}',{status:409});
  await assert.rejects(api('/fixture','PUT',{}),(error:unknown)=>error instanceof ApiError&&!error.outcomeUnknown&&error.status===409);
 }finally{globalThis.fetch=original}
});

test('explicit caller cancellation is preserved instead of reported as a mutation failure',async()=>{
 const original=globalThis.fetch;
 const controller=new AbortController();controller.abort();
 globalThis.fetch=async(_path,init)=>{if(init?.signal?.aborted)throw new DOMException('Cancelled','AbortError');throw new Error('expected cancellation')};
 try{await assert.rejects(api('/fixture','GET',undefined,controller.signal),{name:'AbortError'})}finally{globalThis.fetch=original}
});

test('read and mutation deadlines settle hung requests with different outcome semantics',async t=>{
 const original=globalThis.fetch;
 t.mock.timers.enable({apis:['setTimeout']});
 globalThis.fetch=async(_path,init)=>new Promise((_resolve,reject)=>init?.signal?.addEventListener('abort',()=>reject(new DOMException('Timeout','AbortError'))));
 try{
  const read=api('/fixture');t.mock.timers.tick(20001);
  await assert.rejects(read,(error:unknown)=>error instanceof ApiError&&!error.outcomeUnknown);
  const write=api('/fixture','PUT',{});t.mock.timers.tick(120001);
  await assert.rejects(write,(error:unknown)=>error instanceof ApiError&&error.outcomeUnknown);
 }finally{globalThis.fetch=original;t.mock.timers.reset()}
});
