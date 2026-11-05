import assert from 'node:assert/strict';
import test from 'node:test';
import {quotaBytes, usageGB} from '../src/features/users/usage.ts';
test('packages use decimal GB without accidental unlimited or invalid quota', () => {
 assert.equal(quotaBytes('1'), 1_000_000_000);
 assert.equal(quotaBytes('0,125'), 125_000_000);
 for (const value of ['0','-1','','NaN','Infinity','1e3','0.0001','9000001']) assert.equal(quotaBytes(value),undefined,value);
 assert.match(usageGB(1_000_000_000),/1 ГБ/);
});
