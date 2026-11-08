import {test} from 'node:test';
import assert from 'node:assert/strict';
import {trafficFresh,activityText,trafficHistory,trafficNow,type UserTraffic} from '../src/features/users/traffic.ts';
import {chartPath} from '../src/shared/traffic-chart.ts';
const item:UserTraffic={user_id:1,at:100,upload_bps:0,download_bps:20,window_seconds:5,segment:1,last_activity_at:100,status:'fresh',online:null};
test('user traffic freshness never turns stale, failed or missing data into zero',()=>{
 assert.equal(trafficFresh(item,105),true);assert.equal(trafficFresh(item,125),false);assert.equal(trafficFresh(item,90),false);assert.equal(activityText(undefined),'Нет данных');assert.equal(activityText(item,true),'Нет данных');
});
test('user chart breaks across unknown rates and engine epochs',()=>{
 const points=trafficHistory([{...item,at:100},{...item,at:160},{...item,at:220,download_bps:null,upload_bps:null},{...item,at:280,segment:2}]);
 const path=chartPath(points,'rx_per_second',100);assert.equal((path.match(/M/g)??[]).length,2);assert.equal((path.match(/L/g)??[]).length,1);
});

test('freshness uses server time even when the browser clock differs',()=>{
 const now=trafficNow(100,1000000,1005000);assert.equal(now,105);assert.equal(trafficFresh(item,now),true);assert.equal(activityText(item,false,now),'Есть трафик');
});
