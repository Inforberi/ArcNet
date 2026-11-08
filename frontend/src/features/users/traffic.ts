export type TrafficPoint={at:number;upload_bps:number|null;download_bps:number|null;window_seconds:number;segment:number};
export type UserTraffic=TrafficPoint&{user_id:number;last_activity_at:number;status:string;online:boolean|null};
export type TrafficDetail={current:UserTraffic;history:TrafficPoint[];complete_coverage:boolean};
export type TrafficReply={items:UserTraffic[];detail?:TrafficDetail;sampled_at:number;fresh_for:number;poll_seconds:number;source:string;complete_coverage:boolean;history_persistent:boolean;history_seconds:number};
export function trafficNow(sampledAt:number,receivedAt:number,now=Date.now()){return sampledAt+Math.max(0,now-receivedAt)/1000}
export function trafficFresh(item:UserTraffic|undefined,now=Date.now()/1000){return !!item&&item.status==='fresh'&&now+5>=item.at&&now-item.at<=20}
export function activityText(item:UserTraffic|undefined,failed=false,now?:number){
 if(failed||!item)return 'Нет данных';
 if(!trafficFresh(item,now))return item.status==='collecting'?'Первый замер':'Нет данных';
 return (item.upload_bps??0)+(item.download_bps??0)>0?'Есть трафик':'Нет трафика в замере';
}
export function trafficHistory(points:TrafficPoint[]){return points.map(p=>({received_at:p.at,snapshot:{epoch:String(p.segment),rx_per_second:p.download_bps,tx_per_second:p.upload_bps}}))}
