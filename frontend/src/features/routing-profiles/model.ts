export type Action = 'proxy' | 'direct' | 'block';
export type RoutingProfile = {
  id: string; name: string; enabled: boolean; default_action: Action; order: Action[];
  proxy: string[]; direct: string[]; block: string[];
  domain_strategy: 'AsIs' | 'IPIfNonMatch' | 'IPOnDemand'; remote_dns: string; direct_dns: string;
  version: number; updated_at: number;
};
export const actionTitles: Record<Action, string> = {proxy: 'Через VPN', direct: 'Напрямую', block: 'Блокировать'};
export const emptyProfile = (): RoutingProfile => ({id: '', name: 'Мой профиль', enabled: false, default_action: 'proxy', order: ['block', 'proxy', 'direct'], proxy: [], direct: [], block: [], domain_strategy: 'AsIs', remote_dns: 'https://cloudflare-dns.com/dns-query', direct_dns: 'https://dns.google/dns-query', version: 0, updated_at: 0});
export const textEntries = (text: string) => text.split(/\r?\n/).map(value => value.trim()).filter(value => value && !value.startsWith('#'));
export const countEntries = (profile: RoutingProfile) => profile.proxy.length + profile.direct.length + profile.block.length;
export const moveGroup = (profile: RoutingProfile, index: number, direction: number): RoutingProfile => {
  const next = index + direction;
  if (index < 0 || index >= profile.order.length || next < 0 || next >= profile.order.length) return profile;
  const order = [...profile.order]; [order[index], order[next]] = [order[next], order[index]];
  return {...profile, order};
};
export const withoutMetadata = (profile: RoutingProfile): RoutingProfile => ({...profile, id: '', version: 0, updated_at: 0, order: [...profile.order], proxy: [...profile.proxy], direct: [...profile.direct], block: [...profile.block]});
