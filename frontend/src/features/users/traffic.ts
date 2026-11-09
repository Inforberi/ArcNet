export type TrafficPoint={at:number;upload_bps:number|null;download_bps:number|null;window_seconds:number;segment:number};
export type PresenceSource={source:string;node_id?:string;connection_id?:string;state:string;observed_at:number;last_handshake_at?:number};
export type Presence={state:string;observed_at:number;last_activity_at?:number;complete_coverage:boolean;sources:PresenceSource[]};
export type UserTraffic=TrafficPoint&{user_id:number;last_activity_at:number;status:string;online:boolean|null;presence?:Presence};
export type TrafficDetail={current:UserTraffic;history:TrafficPoint[];complete_coverage:boolean};
export type TrafficReply={items:UserTraffic[];detail?:TrafficDetail;sampled_at:number;fresh_for:number;poll_seconds:number;source:string;complete_coverage:boolean;history_persistent:boolean;history_seconds:number;presence_summary?:{online:number;known:number;total:number;expires_at:number}};
export function presenceState(item:UserTraffic|undefined,failed=false,now=Date.now()/1000):string {
 const p=item?.presence;if(!p)return 'unknown';if(failed)return p.observed_at?'stale':'unknown';
 const fresh=(s:PresenceSource)=>s.observed_at>0&&s.observed_at<=now+5&&now-s.observed_at<=20&&s.state!=='stale';
 if(p.sources.some(s=>fresh(s)&&s.state==='online'))return 'online';
 if((p.last_activity_at&&p.last_activity_at<=now+5&&now-p.last_activity_at<=60)||p.sources.some(s=>fresh(s)&&s.last_handshake_at&&s.last_handshake_at<=now+5&&now-s.last_handshake_at<=180))return 'recent';
 if(p.sources.some(s=>s.observed_at>0&&!fresh(s)))return 'stale';
 if(p.sources.length&&p.sources.every(s=>fresh(s)&&s.state==='offline'))return 'offline';
 return 'unknown';
}
export function presenceText(item:UserTraffic|undefined,failed=false,now?:number):string {return ({online:'Онлайн',recent:'Был недавно',offline:'Не в сети',unknown:'Нет данных',stale:'Данные устарели'} as Record<string,string>)[presenceState(item,failed,now)]}
export function trafficNow(sampledAt:number,receivedAt:number,now=Date.now()){return sampledAt+Math.max(0,now-receivedAt)/1000}
export function trafficFresh(item:UserTraffic|undefined,now=Date.now()/1000){return !!item&&item.status==='fresh'&&now+5>=item.at&&now-item.at<=20}
export function activityText(item:UserTraffic|undefined,failed=false,now?:number){
 if(failed||!item)return 'Нет данных';
 if(!trafficFresh(item,now))return item.status==='collecting'?'Первый замер':'Нет данных';
 return (item.upload_bps??0)+(item.download_bps??0)>0?'Есть трафик':'Нет трафика в замере';
}
export function trafficHistory(points:TrafficPoint[]){return points.map(p=>({received_at:p.at,snapshot:{epoch:String(p.segment),rx_per_second:p.download_bps,tx_per_second:p.upload_bps}}))}
