import test from 'node:test';
import assert from 'node:assert/strict';
import {changeDraft, connectDraft, draftError, graphError, moveBackup, newDraft} from '../src/features/routing/model.ts';

const cascade = () => ({...newDraft(), name: 'Route', mode: 'cascade' as const, entry_node: 'a', primary_exit: 'b', backup_exits: ['c', 'd']});
test('changing entry or exit removes hidden conflicting backups', () => {
  assert.deepEqual(changeDraft(cascade(), {primary_exit: 'c'}).backup_exits, ['d']);
  const entry = changeDraft(cascade(), {entry_node: 'c'});
  assert.deepEqual(entry.backup_exits, ['d']);
  assert.equal(changeDraft(entry, {entry_node: 'b'}).primary_exit, '');
  const direct = changeDraft(entry, {mode: 'direct'});
  assert.equal(direct.primary_exit, ''); assert.deepEqual(direct.backup_exits, []);
});
test('graph gestures enforce distinct nodes, reserve limit and current entry', () => {
  const p = cascade();
  assert.equal(connectDraft(p, 'a', 'a', 'primary'), null);
  assert.equal(connectDraft(p, 'x', 'c', 'backup'), null);
  assert.equal(connectDraft(p, 'a', 'b', 'backup'), null);
  assert.equal(connectDraft(p, 'a', 'c', 'backup'), null);
  const full = {...p, backup_exits: ['c','d','e','f']};
  assert.equal(connectDraft(full, 'a', 'g', 'backup'), null);
  const reversed = connectDraft(p, 'c', 'a', 'primary')!;
  assert.equal(reversed.entry_node, 'c'); assert.equal(reversed.primary_exit, 'a');
  assert.deepEqual(reversed.backup_exits, ['d']);
});
test('cycle validation includes other drafts and all reserves', () => {
  const existing = {...cascade(), id: 'first'};
  const next = {...newDraft(), id: 'second', name: 'Reverse', mode: 'cascade' as const, entry_node: 'c', primary_exit: 'a'};
  assert.notEqual(graphError([existing], next), '');
  assert.equal(graphError([existing], {...existing, primary_exit: 'd', backup_exits: []}), '');
});
test('reserve order is explicit and transitions do not mutate saved data', () => {
  const p = cascade();
  assert.deepEqual(moveBackup(p, 1, -1).backup_exits, ['d', 'c']);
  assert.deepEqual(p.backup_exits, ['c', 'd']);
  assert.equal(moveBackup(p, 0, -1), p);
});
test('missing nodes, invalid thresholds, duplicates and excess reserves block save', () => {
  const p = cascade(); const ids = ['a','b','c','d','e','f','g'];
  assert.equal(draftError(p, ids, []), '');
  for (const bad of [{...p, entry_node:'missing'}, {...p, failures_to_switch:1.5}, {...p, cooldown_seconds:NaN}, {...p, backup_exits:['c','c']}, {...p, backup_exits:['c','d','e','f','g']}]) assert.notEqual(draftError(bad, ids, []), '');
});
