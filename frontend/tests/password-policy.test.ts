import {test} from 'node:test';
import assert from 'node:assert/strict';
import {passwordError} from '../src/shared/password-policy.ts';
test('new password policy counts Unicode code points, preserves spaces and bounds UTF-8 bytes',()=>{
 for(const value of ['12345678','        ',' Пароль ','😀'.repeat(8),'😀'.repeat(64),'e\u0301'.repeat(4)])assert.equal(passwordError(value),undefined);
 for(const value of ['1234567','😀'.repeat(4),'😀'.repeat(65),'x'.repeat(257),'12345678\n','12345678\0','\ud800'.repeat(8)])assert.ok(passwordError(value));
});
