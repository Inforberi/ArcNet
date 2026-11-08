import {test} from 'node:test';
import assert from 'node:assert/strict';
import {versionLabel,releaseLabel} from '../src/features/settings/version-label.ts';
test('release numbers are readable without changing package identity',()=>{
 assert.equal(versionLabel('0.7.0-stage5-a8'),'0.7.0');
 assert.equal(versionLabel('v1.2.3'),'1.2.3');
 assert.equal(versionLabel('unknown'),'unknown');
 assert.equal(releaseLabel('0.7.0-stage5-a8',['0.7.0-stage5-a7','0.7.0-stage5-a8']),'0.7.0 · A8');
 assert.equal(releaseLabel('0.7.0-stage5-a8',['0.7.0-stage5-a8']),'0.7.0');
 assert.equal(releaseLabel('0.7.0-custom',['0.7.0-other','0.7.0-custom']),'0.7.0-custom');
});
