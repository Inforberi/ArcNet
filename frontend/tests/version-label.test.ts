import {test} from 'node:test';
import assert from 'node:assert/strict';
import {versionLabel,releaseLabel,publishedLabel} from '../src/features/settings/version-label.ts';
test('release numbers are readable without changing package identity',()=>{
 assert.equal(versionLabel('0.7.0-stage5-a8'),'0.7.0');
 assert.equal(versionLabel('v1.2.3'),'1.2.3');
 assert.equal(versionLabel('unknown'),'unknown');
 assert.equal(releaseLabel('0.7.0-stage5-a8',['0.7.0-stage5-a7','0.7.0-stage5-a8']),'0.7.0');
 assert.equal(releaseLabel('0.7.0-stage5-a8',['0.7.0-stage5-a8']),'0.7.0');
 assert.equal(versionLabel('0.7.1-rc.1'),'0.7.1-rc.1');
 assert.equal(versionLabel('0.7.1+build'),'0.7.1');
 assert.equal(versionLabel('0.6.5-stage4'),'0.6.5');
 assert.equal(publishedLabel(undefined),'Дата не указана');
 assert.equal(publishedLabel('invalid'),'Дата не указана');
 assert.notEqual(publishedLabel('2026-11-09T12:00:00Z'),'Дата не указана');
});
