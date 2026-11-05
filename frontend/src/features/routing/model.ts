export type RoutePolicy = {
  id: string;
  name: string;
  mode: 'direct' | 'cascade';
  entry_node: string;
  primary_exit?: string;
  backup_exits: string[];
  failure_mode: 'closed' | 'open';
  failures_to_switch: number;
  successes_to_recover: number;
  cooldown_seconds: number;
  version: number;
  updated_at: number;
};
export type RouteEdge = {from: string; to: string; backup: boolean; policy_id: string};
export type RouteList = {items: RoutePolicy[]; edges: RouteEdge[]; stage: string; publication_enabled: boolean};
export type ManagedNode = {id: string; name: string; online: boolean; control: {vpn_probe: string; revision_status: string}};
export type RoutePreview = {valid: boolean; edges: RouteEdge[]; changes: {field: string; before: string; after: string}[]; side_effects: boolean};

export const newDraft = (): RoutePolicy => ({
  id: '', name: '', mode: 'direct', entry_node: 'local', primary_exit: '', backup_exits: [],
  failure_mode: 'closed', failures_to_switch: 3, successes_to_recover: 3, cooldown_seconds: 60,
  version: 0, updated_at: 0,
});
export const nodeTitle = (id: string, nodes: ManagedNode[]) =>
  id === 'local' ? 'Локальный VPS' : nodes.find(node => node.id === id)?.name ?? `Недоступная нода (${id})`;

// One transition function serves the selects, node actions and graph gestures.
export const changeDraft = (draft: RoutePolicy, patch: Partial<RoutePolicy>): RoutePolicy => {
  const next = {...draft, ...patch};
  if (next.mode === 'direct') return {...next, primary_exit: '', backup_exits: []};
  if (next.primary_exit === next.entry_node) next.primary_exit = '';
  next.backup_exits = [...new Set(next.backup_exits)].filter(id => id !== next.entry_node && id !== next.primary_exit);
  return next;
};

export const policyEdges = (p: RoutePolicy): RouteEdge[] => p.mode === 'direct' ? [] : [
  ...(p.primary_exit ? [{from: p.entry_node, to: p.primary_exit, backup: false, policy_id: p.id}] : []),
  ...p.backup_exits.map(to => ({from: p.entry_node, to, backup: true, policy_id: p.id})),
];

export const graphError = (policies: RoutePolicy[], draft: RoutePolicy): string => {
  const edges = [...policies.filter(p => p.id !== draft.id).flatMap(policyEdges), ...policyEdges(draft)];
  const adjacency = new Map<string, string[]>();
  for (const edge of edges) adjacency.set(edge.from, [...(adjacency.get(edge.from) ?? []), edge.to]);
  const visiting = new Set<string>();
  const visited = new Set<string>();
  const walk = (id: string): boolean => {
    if (visiting.has(id)) return true;
    if (visited.has(id)) return false;
    visiting.add(id);
    if ((adjacency.get(id) ?? []).some(walk)) return true;
    visiting.delete(id);
    visited.add(id);
    return false;
  };
  return [...adjacency.keys()].some(walk) ? 'Связь создаёт цикл с другим основным или резервным маршрутом.' : '';
};

export const draftError = (draft: RoutePolicy, ids: string[], policies: RoutePolicy[]): string => {
  if (!draft.name.trim() || [...draft.name.trim()].length > 80 || /[\r\n\0]/.test(draft.name)) return 'Укажите название маршрута: от 1 до 80 символов.';
  if (!ids.includes(draft.entry_node)) return 'Входная нода больше недоступна. Выберите другую.';
  if (draft.mode === 'cascade') {
    if (!draft.primary_exit || !ids.includes(draft.primary_exit) || draft.primary_exit === draft.entry_node) return 'Выберите выходную ноду, отличную от входной.';
    if (draft.backup_exits.length > 4) return 'Можно выбрать не более четырёх резервных выходов.';
    if (draft.backup_exits.some(id => !ids.includes(id) || id === draft.entry_node || id === draft.primary_exit) || new Set(draft.backup_exits).size !== draft.backup_exits.length) return 'Проверьте резервные выходы: ноды должны существовать и не повторяться.';
  }
  if (![draft.failures_to_switch, draft.successes_to_recover].every(n => Number.isInteger(n) && n >= 1 && n <= 20)) return 'Пороги переключения и восстановления: целые числа от 1 до 20.';
  if (!Number.isInteger(draft.cooldown_seconds) || draft.cooldown_seconds < 0 || draft.cooldown_seconds > 86400) return 'Время ожидания: целое число от 0 до 86400 секунд.';
  return graphError(policies, draft);
};

export const connectDraft = (draft: RoutePolicy, from: string, to: string, kind: 'primary' | 'backup'): RoutePolicy | null => {
  if (from === to) return null;
  if (kind === 'backup') {
    if (draft.mode !== 'cascade' || !draft.primary_exit || from !== draft.entry_node || to === draft.primary_exit || draft.backup_exits.includes(to) || draft.backup_exits.length >= 4) return null;
    return changeDraft(draft, {backup_exits: [...draft.backup_exits, to]});
  }
  return changeDraft(draft, {mode: 'cascade', entry_node: from, primary_exit: to});
};

export const moveBackup = (draft: RoutePolicy, index: number, delta: number): RoutePolicy => {
  const target = index + delta;
  if (index < 0 || index >= draft.backup_exits.length || target < 0 || target >= draft.backup_exits.length) return draft;
  const backups = [...draft.backup_exits];
  [backups[index], backups[target]] = [backups[target], backups[index]];
  return {...draft, backup_exits: backups};
};
