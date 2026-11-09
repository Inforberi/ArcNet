export interface Session {
  authenticated: boolean;
  csrf: string;
  username: string;
}

export interface VPNConfig {
 server_options?:ServerOptions;
 fingerprint?: string;
  configured: boolean;
  enabled?: boolean;
  revision?: string;
  address?: string;
  port?: number;
  target?: string;
  sni?: string;
  public_key?: string;
  short_id?: string;
}

export type ProtocolKind = 'vless-xhttp' | 'hysteria2' | 'wireguard' | 'amneziawg';
export type ServerOptions={xhttp_host?:string;flow?:string;salamander?:boolean;up_mbps?:number;down_mbps?:number;sniffing?:boolean;udp_idle_seconds?:number;quic_idle_seconds?:number};
export type ClientOptions={xhttp_mode?:string;mtu?:number;dns?:string;keepalive?:number};

export interface ProtocolProfile {
 id?:string;
 server_options?:ServerOptions;
	client_options?:ClientOptions;
 fingerprint?: string;
  revision?: string;
  kind: ProtocolKind|'vless-reality';
  address: string;
  port: number;
  sni?: string;
  target?: string;
  public_key?: string;
  short_id?: string;
  path?: string;
  enabled: boolean;
}

export interface ProtocolHealth {
  kind: string;
  enabled: boolean;
  state: 'active' | 'failed' | 'inactive' | 'activating' | 'deactivating' | 'unknown' | 'disabled' | 'unverified';
  evidence: string;
}

export interface ConnectionProfile {
  kind: string;
  label: string;
  format: 'uri' | 'conf';
  link?: string;
  configuration?: string;
}

export type UserPreferences={version:number;group:string;comment:string;title:string;access_mode:string;direct_kind:string;allowed_kinds:string[]|null};
export interface VPNUser {
 preferences?:UserPreferences;
  devices_managed: boolean;
  id: number;
  name: string;
  uuid: string;
  enabled: boolean;
  created_at: number;
}

export interface MetricSnapshot {
  memory_total: number;
  memory_available: number;
  uptime_seconds: number;
  load_1: number;
}

export interface Overview {
  system: MetricSnapshot;
  users: number;
  enabled_users: number;
  xray: string;
  nodes: number;
  mode: string;
}

export interface Subscription {
 title?: string;
 link_ids: string[];
  url: string;
  link: string;
  links: string[];
}

export interface AuditEvent {
  id: string;
  time: string;
  action: string;
  details: string;
}

let csrfToken = '';

export function setCSRF(token: string): void {
  csrfToken = token;
}

export class ApiError extends Error {
  readonly status:number;
  readonly outcomeUnknown:boolean;
  readonly retryAt:number;
  readonly code?:string;
  readonly stage?:string;
  readonly outcome?:string;
  constructor(
    status: number,
    message: string,
    outcomeUnknown = false,
    retryAt = 0,
    metadata:{code?:string;stage?:string;outcome?:string} = {},
  ) {
    super(message);
    this.name = 'ApiError';
    this.status=status;
    this.outcomeUnknown=outcomeUnknown;
    this.retryAt=retryAt;
    this.code=metadata.code;this.stage=metadata.stage;this.outcome=metadata.outcome;
  }
}

export function retryAfter(raw:string|null,now=Date.now()):number {
 if(!raw)return 0;
 const seconds=Number(raw),at=Number.isFinite(seconds)&&seconds>=0?now+seconds*1000:Date.parse(raw);
 return Number.isFinite(at)&&at>now?Math.min(at,now+86400000):0;
}

export function retryQuery(failureCount:number,error:Error):boolean {
  return failureCount<1&&!(error instanceof ApiError&&error.status>=400&&error.status<500);
}

async function request(path:string,init:RequestInit,signal?:AbortSignal):Promise<{response:Response;text:string}>{
 const mutation=init.method!=='GET';
 const controller=new AbortController();
 const abort=()=>controller.abort();
 if(signal?.aborted)controller.abort();else signal?.addEventListener('abort',abort,{once:true});
 const timer=setTimeout(abort,mutation?120000:20000);
 try{const response=await fetch(path,{...init,signal:controller.signal});return {response,text:await response.text()}}
 catch(error){
  if(signal?.aborted&&!mutation)throw error;
  throw new ApiError(0,mutation?'Ответ сервера не получен. Результат операции неизвестен. Проверьте текущее состояние перед повтором.':'Не удалось получить ответ сервера. Проверьте соединение и повторите загрузку.',mutation);
 }finally{clearTimeout(timer);signal?.removeEventListener('abort',abort)}
}

export async function api<T>(
  path: string,
  method: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE' = 'GET',
  body?: unknown,
  signal?: AbortSignal,
): Promise<T> {
  const headers: Record<string, string> = { Accept: 'application/json' };
  if (method !== 'GET') headers['X-CSRF-Token'] = csrfToken;
  if (body !== undefined) headers['Content-Type'] = 'application/json';

  const {response,text:responseText} = await request(path, {
    method,
    headers,
    credentials: 'same-origin',
    redirect: 'error',
    body: body === undefined ? undefined : JSON.stringify(body),
  },signal);

  if (response.status === 401 && path !== '/api/session' && path !== '/api/login') {
    window.dispatchEvent(new Event('arcvpn:unauthorized'));
  }

  if (response.status === 204) {
    if (!response.ok) throw new ApiError(response.status, `HTTP ${response.status}`);
    return undefined as T;
  }

  let result: unknown;
  try {
    result = JSON.parse(responseText);
  } catch {
    throw new ApiError(response.status, method==='GET'?`Некорректный ответ сервера: HTTP ${response.status}`:'Ответ сервера не распознан. Результат операции неизвестен; проверьте текущее состояние перед повтором.',method!=='GET');
  }

  if (!response.ok) {
    const message =
      result && typeof result === 'object' && 'error' in result && typeof result.error === 'string'
        ? result.error
        : `HTTP ${response.status}`;
    const value=result as Record<string,unknown>|null;
    const outcome=typeof value?.outcome==='string'&&['rejected','applied','saved_pending','rolled_back','unknown'].includes(value.outcome)?value.outcome:undefined;
    const unknown=method!=='GET'&&(outcome==='unknown'||(!outcome&&response.status>=500));
    throw new ApiError(response.status,message,unknown,response.status===429?retryAfter(response.headers.get('Retry-After')):0,{outcome,code:typeof value?.code==='string'?value.code:undefined,stage:typeof value?.stage==='string'?value.stage:undefined});
  }

  return result as T;
}

// Upload a signed package without base64 encoding or buffering it in JavaScript.
// The root helper performs the actual Ed25519 verification before any update.
export async function uploadSignedUpdate(file: File, signal?: AbortSignal): Promise<{version:string;verified:boolean}> {
  const {response,text} = await request('/api/system/update/upload', {
    method: 'POST', credentials:'same-origin', redirect:'error',
    headers: {'Content-Type':'application/octet-stream', 'X-CSRF-Token':csrfToken, Accept:'application/json'},
    body:file,
  },signal);
  if (response.status===401) window.dispatchEvent(new Event('arcvpn:unauthorized'));
  let body:unknown;try{body=JSON.parse(text)}catch{body=null}
  if(!response.ok){
    const message=body&&typeof body==='object'&&'error'in body&&typeof body.error==='string'?body.error:`HTTP ${response.status}`;
    throw new ApiError(response.status,message);
  }
  if(!body||typeof body!=='object'||!('version'in body)||typeof body.version!=='string'||!('verified'in body)||body.verified!==true){
    throw new ApiError(response.status,'Некорректный ответ сервера обновлений');
  }
  return {version:body.version,verified:true};
}
