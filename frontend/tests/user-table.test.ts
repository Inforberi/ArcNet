import {test} from 'node:test';
import assert from 'node:assert/strict';
import {sortUsers,usersCSV} from '../src/features/users/user-table.ts';
const users=[{id:2,name:'User 10',enabled:true,created_at:2,uuid:'must-not-export',preferences:{group:'=1+1',allowed_kinds:['wireguard']}},{id:1,name:'User 2',enabled:false,created_at:1},{id:3,name:'"comma,\nname',enabled:true,created_at:3}];
const packages=new Map([[1,{observed_at:1,upload_bytes:2,download_bytes:3,expires_at:0,quota_bytes:20}],[2,{observed_at:1,upload_bytes:4,download_bytes:6,expires_at:100,quota_bytes:0}]]);
test('user sorting is numeric, deterministic and keeps unknown telemetry last',()=>{
 assert.deepEqual(sortUsers(users as never,packages as never,'name').map(u=>u.id),[3,1,2]);
 assert.deepEqual(sortUsers(users as never,packages as never,'traffic',true).map(u=>u.id),[2,1,3]);
 assert.deepEqual(sortUsers(users as never,packages as never,'expires').map(u=>u.id),[2,1,3]);
 assert.deepEqual(users.map(u=>u.id),[2,1,3]);
});
test('CSV exports only table fields and escapes formulas, quotes and line breaks',()=>{
 const csv=usersCSV(users as never,packages as never);
 assert.ok(csv.startsWith('\uFEFF'));assert.ok(csv.includes('"\'=1+1"'));assert.ok(csv.includes('"""comma,\nname"'));
 assert.equal(csv.includes('must-not-export'),false);
 assert.ok(csv.includes('"4","0"')); // actual byte counters, never rounded to zero GB
});
