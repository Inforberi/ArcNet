import {test} from 'node:test';
import assert from 'node:assert/strict';
import {interfaceModeKey,parseInterfaceMode,readInterfaceMode,saveInterfaceMode} from '../src/shared/interface-mode.ts';

test('interface mode defaults safely and accepts only known values',()=>{
 for(const value of [null,'','simple','light','true','<script>'])assert.equal(parseInterfaceMode(value),'simple');
 assert.equal(parseInterfaceMode('advanced'),'advanced');
 assert.equal(readInterfaceMode({getItem:()=>{throw new Error('blocked')}}),'simple');
});
test('interface mode persists without touching other settings',()=>{
 const values=new Map([['foreign','keep']]);
 const storage={getItem:(key:string)=>values.get(key)??null,setItem:(key:string,value:string)=>{values.set(key,value)}};
 assert.equal(saveInterfaceMode(storage,'advanced'),true);
 assert.equal(values.get(interfaceModeKey),'advanced');
 assert.equal(readInterfaceMode(storage),'advanced');
 assert.equal(saveInterfaceMode(storage,'simple'),true);
 assert.equal(readInterfaceMode(storage),'simple');
 assert.equal(values.get('foreign'),'keep');
 assert.equal(saveInterfaceMode({setItem:()=>{throw new Error('blocked')}},'advanced'),false);
});
