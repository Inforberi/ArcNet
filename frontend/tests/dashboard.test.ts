import assert from 'node:assert/strict';
import test from 'node:test';
import {ageText, chartPath, formatBytes, formatPercent, needsAttention, percentUsed, pointX, rateText, type FleetItem, type Observed} from '../src/features/dashboard/model.ts';

test('unknown resources stay unknown and no swap differs from zero usage', () => {
 assert.equal(formatBytes(null), 'Нет данных'); assert.equal(formatPercent(null), 'Нет данных'); assert.equal(rateText(null), 'Нет данных');
 assert.equal(percentUsed(100, 50), 50); assert.equal(percentUsed(0, 0), null); assert.equal(percentUsed(100, 101), null);
 assert.equal(formatPercent(0), '0%'); assert.equal(formatBytes(0), '0 Б');
 assert.equal(ageText(0, 100), 'Не получены'); assert.equal(ageText(200, 100), 'Часы не совпадают');
});
test('traffic chart preserves gaps and epoch changes on a real time axis', () => {
 const point = (time: number, rate: number | null, epoch = 'a') => ({received_at: time, snapshot: {rx_per_second: rate, tx_per_second: rate, epoch}}) as Observed;
 const points = [point(60, 1), point(120, 2), point(300, 3), point(360, null), point(420, 3), point(480, 4, 'b')];
 const path = chartPath(points, 'rx_per_second', 4);
 assert.equal((path.match(/M/g) ?? []).length, 4);
 assert.equal((path.match(/L/g) ?? []).length, 1);
 assert.equal(pointX(points, 1), 600 * 60 / 420);
});
test('attention uses actual failures and stale data without claiming missing VPN proof is a failure', () => {
 const node: FleetItem = {id: 'a', name: 'a', address: '', agent: 'online', xray: 'active', vpn: 'unknown', last_seen: 100, stale: false, legacy: false, cpu_percent: null, memory_percent: null};
 assert.equal(needsAttention(node), false);
 assert.equal(needsAttention({...node, stale: true}), true);
 assert.equal(needsAttention({...node, xray: 'inactive'}), true);
 assert.equal(needsAttention({...node, agent: 'offline'}), true);
});
