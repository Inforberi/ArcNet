export interface Session {
  authenticated: boolean;
  csrf: string;
  username: string;
}

export interface VPNConfig {
  configured: boolean;
  address?: string;
  port?: number;
  target?: string;
  sni?: string;
  public_key?: string;
  short_id?: string;
}

export type ProtocolKind = 'vless-xhttp' | 'hysteria2' | 'wireguard' | 'amneziawg';

export interface ProtocolProfile {
  kind: ProtocolKind;
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

export interface VPNUser {
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
  url: string;
  link: string;
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
  constructor(
    readonly status: number,
    message: string,
  ) {
    super(message);
    this.name = 'ApiError';
  }
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

  const response = await fetch(path, {
    method,
    headers,
    credentials: 'same-origin',
    redirect: 'error',
    body: body === undefined ? undefined : JSON.stringify(body),
    signal,
  });

  if (response.status === 401 && path !== '/api/session' && path !== '/api/login') {
    window.dispatchEvent(new Event('arcvpn:unauthorized'));
  }

  if (response.status === 204) {
    if (!response.ok) throw new ApiError(response.status, `HTTP ${response.status}`);
    return undefined as T;
  }

  const responseText = await response.text();
  let result: unknown;
  try {
    result = JSON.parse(responseText);
  } catch {
    throw new ApiError(response.status, `Некорректный ответ сервера: HTTP ${response.status}`);
  }

  if (!response.ok) {
    const message =
      result && typeof result === 'object' && 'error' in result && typeof result.error === 'string'
        ? result.error
        : `HTTP ${response.status}`;
    throw new ApiError(response.status, message);
  }

  return result as T;
}

// Upload a signed package without base64 encoding or buffering it in JavaScript.
// The root helper performs the actual Ed25519 verification before any update.
export async function uploadSignedUpdate(file: File, signal?: AbortSignal): Promise<{version:string;verified:boolean}> {
  const response = await fetch('/api/system/update/upload', {
    method: 'POST', credentials:'same-origin', redirect:'error',
    headers: {'Content-Type':'application/octet-stream', 'X-CSRF-Token':csrfToken, Accept:'application/json'},
    body:file, signal,
  });
  if (response.status===401) window.dispatchEvent(new Event('arcvpn:unauthorized'));
  const body:unknown=await response.json().catch(()=>null);
  if(!response.ok){
    const message=body&&typeof body==='object'&&'error'in body&&typeof body.error==='string'?body.error:`HTTP ${response.status}`;
    throw new ApiError(response.status,message);
  }
  if(!body||typeof body!=='object'||!('version'in body)||typeof body.version!=='string'||!('verified'in body)||body.verified!==true){
    throw new ApiError(response.status,'Некорректный ответ сервера обновлений');
  }
  return {version:body.version,verified:true};
}
