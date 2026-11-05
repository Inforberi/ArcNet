import assert from 'node:assert/strict';
import test from 'node:test';
import {emptyProfile, moveGroup, textEntries, withoutMetadata} from '../src/features/routing-profiles/model.ts';
import {usableResult, type TargetJob} from '../src/features/targets/model.ts';

test('profile duplication clears identity without mutating saved rules or order', () => {
 const original = {...emptyProfile(), id: 'saved', version: 4, direct: ['example.com']};
 const copy = withoutMetadata(original); copy.direct.push('other.com');
 assert.equal(copy.id, ''); assert.equal(copy.version, 0); assert.deepEqual(original.direct, ['example.com']);
 const reordered = moveGroup(original, 1, -1);
 assert.deepEqual(original.order, ['block', 'proxy', 'direct']); assert.deepEqual(reordered.order, ['proxy', 'block', 'direct']);
 assert.deepEqual(textEntries('# comment\r\n example.com \r\n\n geoip:ru'), ['example.com', 'geoip:ru']);
});

test('SNI selection rejects stale, wrong-node, failed and incompatible reports', () => {
 const candidate = {host: 'example.com', sni: 'example.com'};
 const job: TargetJob = {id: 'a', version: 1, node_id: 'local', state: 'done', expires_at: 120, report: {measured_from: 'local', checked_at: 100, expires_at: 700, recommended: candidate, vpn_verified: false, results: [{candidate, eligible: true, successes: 3, median_ms: 10, p95_ms: 11, checked_at: 100, http_verified: true, vpn_verified: false}]}};
 assert(usableResult(job, 'local', candidate, 110));
 assert.equal(usableResult(job, 'other-node', candidate, 110), undefined);
 assert.equal(usableResult(job, 'local', candidate, 700), undefined);
 assert.equal(usableResult({...job, version: 2}, 'local', candidate, 110), undefined);
 assert.equal(usableResult({...job, state: 'queued'}, 'local', candidate, 110), undefined);
 job.report!.results[0].successes = 2;
 assert.equal(usableResult(job, 'local', candidate, 110), undefined);
});
