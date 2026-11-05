import {memo, useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {ReactFlow, Background, Controls, Handle, MarkerType, Position, applyNodeChanges,
  type Connection, type Edge, type Node, type NodeProps, type NodeChange, type ReactFlowInstance} from '@xyflow/react';
import {Button} from '../../shared/ui';
import {connectDraft, graphError, nodeTitle, type ManagedNode, type RoutePolicy} from './model';
import '@xyflow/react/dist/style.css';

type CardData = {name: string; role: string; status: string; endpoint?: boolean};
type Card = Node<CardData, 'server'>;
const ServerCard = memo(({data}: NodeProps<Card>) => <div className="route-node-card">
  <Handle type="target" position={Position.Left} id="in" isConnectable={!data.endpoint} className={data.endpoint ? 'route-endpoint-handle' : ''}/>
  <span className="route-node-role">{data.role}</span>
  <strong title={data.name}>{data.name}</strong>
  <small>{data.status}</small>
  <Handle type="source" position={Position.Right} id="out" isConnectable={!data.endpoint} className={data.endpoint ? 'route-endpoint-handle' : ''}/>
</div>);
const nodeTypes = {server: ServerCard};
const ariaLabelConfig = {
  'controls.ariaLabel': 'Управление схемой',
  'controls.zoomIn.ariaLabel': 'Увеличить схему',
  'controls.zoomOut.ariaLabel': 'Уменьшить схему',
  'controls.fitView.ariaLabel': 'Показать всю схему',
  'node.a11yDescription.default': 'Выберите сервер. Для перемещения используйте стрелки клавиатуры. Назначить роль можно кнопками под схемой.',
  'edge.a11yDescription.default': 'Выберите связь и нажмите Delete для удаления из черновика.',
};

export default function RouteCanvas({draft, nodes, policies, disabled, onChange}: {
  draft: RoutePolicy; nodes: ManagedNode[]; policies: RoutePolicy[]; disabled: boolean;
  onChange: (next: RoutePolicy) => void;
}) {
  const [kind, setKind] = useState<'primary' | 'backup'>('primary');
  const [selected, setSelected] = useState('');
  const [search, setSearch] = useState('');
  const allIDs = useMemo(() => ['local', ...nodes.map(node => node.id)], [nodes]);
  const exits = useMemo(() => draft.mode === 'cascade' ? [draft.primary_exit, ...draft.backup_exits].filter((id): id is string => !!id) : [], [draft.mode, draft.primary_exit, draft.backup_exits]);
  const ids = useMemo(() => [...new Set([
    draft.entry_node, ...exits,
    ...allIDs.filter(id => nodeTitle(id, nodes).toLocaleLowerCase('ru').includes(search.trim().toLocaleLowerCase('ru'))).slice(0, 8),
  ])].filter(id => allIDs.includes(id)), [draft.entry_node, exits, allIDs, nodes, search]);
  const [cards, setCards] = useState<Card[]>([]);
  const [instance, setInstance] = useState<ReactFlowInstance<Card> | null>(null);
  const layoutKey = JSON.stringify([draft.entry_node, exits, ids]);
  const previousLayout = useRef('');
  useEffect(() => {
    const other = ids.filter(id => id !== draft.entry_node && !exits.includes(id));
    const next: Card[] = ids.map(id => {
      const exitIndex = exits.indexOf(id);
      const spareIndex = other.indexOf(id);
      const remote = nodes.find(node => node.id === id);
      const role = id === draft.entry_node ? 'Вход' : exitIndex === 0 ? 'Основной выход' : exitIndex > 0 ? `Резерв ${exitIndex}` : 'Не участвует';
      return {id, type: 'server', data: {
        name: nodeTitle(id, nodes), role,
        status: remote ? `${remote.online ? 'Агент на связи' : 'Агент не на связи'} · VPN: ${remote.control.vpn_probe === 'pass' ? 'внешняя проверка PASS' : 'не подтверждён'}` : 'Локальная нода · VPN не проверен',
      }, position: id === draft.entry_node ? {x: 250, y: 60} : exitIndex >= 0 ? {x: 580, y: 60 + exitIndex * 160} : {x: 250 + (spareIndex % 2) * 330, y: Math.max(240, exits.length * 160 + 70) + Math.floor(spareIndex / 2) * 160},
      className: id === draft.entry_node ? 'route-node-entry' : exitIndex >= 0 ? 'route-node-exit' : '',
      ariaLabel: `${nodeTitle(id, nodes)}: ${role}`, deletable: false};
    });
    next.unshift({id: '$client', type: 'server', position: {x: -70, y: 60}, data: {name: 'VPN-клиент', role: 'Начало маршрута', status: 'Проектируемый путь', endpoint: true}, connectable: false, deletable: false, draggable: false, selectable: false});
    next.push({id: '$internet', type: 'server', position: {x: exits.length ? 910 : 580, y: 60}, data: {name: 'Интернет', role: 'Конечный выход', status: 'Egress ещё не проверен', endpoint: true}, connectable: false, deletable: false, draggable: false, selectable: false});
    const relayout = previousLayout.current !== layoutKey;
    previousLayout.current = layoutKey;
    setCards(previous => next.map(card => {
      const old = previous.find(item => item.id === card.id);
      return {...card, position: !relayout && old ? old.position : card.position, selected: old?.selected ?? false};
    }));
    // Re-layout only when participants or roles change, never on telemetry polling.
  }, [layoutKey, ids, nodes, draft.entry_node, exits]);
  useEffect(() => {
    // Fit after React has committed the role-based layout. Ordinary telemetry,
    // selection and manual node moves leave the user's viewport untouched.
    const frame = requestAnimationFrame(() => {void instance?.fitView({padding: 0.15, maxZoom: 0.85});});
    return () => cancelAnimationFrame(frame);
  }, [layoutKey, instance]);

  const edges = useMemo<Edge[]>(() => {
    const edge = (id: string, source: string, target: string, label: string, backup = false, editable = false): Edge => ({
      id, source, target, sourceHandle: 'out', targetHandle: 'in',
      label, type: 'smoothstep', animated: false, deletable: editable, focusable: editable,
      selectable: editable, reconnectable: false,
      ariaLabel: label, markerEnd: {type: MarkerType.ArrowClosed},
      style: {stroke: backup ? 'var(--amber)' : 'var(--accent)', strokeWidth: 2, strokeDasharray: '7 5'},
      labelStyle: {fill: 'var(--text)', fontSize: 12}, labelBgStyle: {fill: 'var(--surface)'},
    });
    const result = [edge('client', '$client', draft.entry_node, 'Черновик')];
    if (draft.mode === 'direct') result.push(edge('internet', draft.entry_node, '$internet', 'Прямой выход'));
    else if (draft.primary_exit) {
      result.push(edge('primary', draft.entry_node, draft.primary_exit, 'Основной', false, true));
      result.push(edge('internet', draft.primary_exit, '$internet', 'Выход в интернет'));
      draft.backup_exits.forEach((id, i) => {
        result.push(edge(`backup:${id}`, draft.entry_node, id, `Резерв ${i + 1}`, true, true));
        result.push(edge(`internet:${id}`, id, '$internet', `Выход резерва ${i + 1}`, true));
      });
    }
    return result;
  }, [draft.entry_node, draft.mode, draft.primary_exit, draft.backup_exits]);

  const connectionPolicy = useCallback((connection: Connection | Edge) => {
    if (disabled || !ids.includes(connection.source) || !ids.includes(connection.target)) return null;
    const next = connectDraft(draft, connection.source, connection.target, kind);
    return next && !graphError(policies, next) ? next : null;
  }, [disabled, ids, draft, kind, policies]);
  const connect = useCallback((connection: Connection) => {
    const next = connectionPolicy(connection);
    if (next) onChange(next);
  }, [connectionPolicy, onChange]);
  const changes = useCallback((items: NodeChange<Card>[]) => setCards(current => applyNodeChanges(items, current)), []);
  const selectedName = selected ? nodeTitle(selected, nodes) : '';
  const backupAllowed = draft.mode === 'cascade' && !!draft.primary_exit && selected !== draft.entry_node && selected !== draft.primary_exit && !draft.backup_exits.includes(selected) && draft.backup_exits.length < 4;

  return <div className="route-canvas-editor">
    <div className="route-toolbar">
      <label className="field"><span>Новая стрелка</span><select aria-label="Новая стрелка" value={kind} disabled={disabled} onChange={e => setKind(e.target.value as typeof kind)}><option value="primary">Основной маршрут</option><option value="backup" disabled={!draft.primary_exit}>Резервный выход</option></select></label>
      <p className="muted">Соедините правую точку входа с левой точкой выхода. Основная стрелка задаёт вход и выход; резервная добавляется от текущего входа.</p>
    </div>
    {allIDs.length > 8 && <label className="field"><span>Найти сервер для схемы</span><input value={search} onChange={event => setSearch(event.target.value)} placeholder="Название сервера"/><small>Участники маршрута и до 8 серверов по поиску. Любую ноду также можно выбрать в форме.</small></label>}
    <div className="route-canvas" aria-label="Редактор схемы черновика">
      <ReactFlow<Card> nodes={cards} edges={edges} nodeTypes={nodeTypes} onNodesChange={changes}
        onInit={setInstance}
        onConnect={connect} isValidConnection={connection => !!connectionPolicy(connection)}
        onNodeClick={(_, node) => {if (ids.includes(node.id)) setSelected(node.id);}}
        onSelectionChange={({nodes: selectedNodes}) => setSelected(selectedNodes.find(node => ids.includes(node.id))?.id ?? '')}
        onEdgesDelete={deleted => {
          if (disabled) return;
          if (deleted.some(edge => edge.id === 'primary')) onChange({...draft, mode: 'direct', primary_exit: '', backup_exits: []});
          else onChange({...draft, backup_exits: draft.backup_exits.filter(id => !deleted.some(edge => edge.id === `backup:${id}`))});
        }}
        nodesConnectable={!disabled} edgesReconnectable={false} nodesDraggable={!disabled}
        deleteKeyCode={disabled ? null : ['Backspace', 'Delete']} fitView minZoom={0.15} maxZoom={1.5}
        fitViewOptions={{padding: 0.15, maxZoom: 0.85}} zoomOnScroll={false} preventScrolling={false}
        ariaLabelConfig={ariaLabelConfig} colorMode="system">
        <Background color="var(--stroke)" gap={24}/><Controls showInteractive={false}/>
      </ReactFlow>
    </div>
    <p className="muted route-legend">Все линии — черновик. Синий: основной путь. Жёлтый с подписью «Резерв»: запасной выход. Управляющие соединения здесь не показаны.</p>
    {selected && ids.includes(selected) && <div className="route-selection" aria-label="Действия выбранной ноды"><strong>{selectedName}</strong><div className="flex">
      <Button type="button" disabled={disabled || selected === draft.entry_node} onClick={() => onChange({...draft, entry_node: selected, primary_exit: draft.primary_exit === selected ? '' : draft.primary_exit, backup_exits: draft.backup_exits.filter(id => id !== selected)})}>Сделать входом</Button>
      <Button type="button" disabled={disabled || selected === draft.entry_node} onClick={() => {const next = connectDraft(draft, draft.entry_node, selected, 'primary'); if (next) onChange(next);}}>Сделать основным выходом</Button>
      <Button type="button" disabled={disabled || !backupAllowed} onClick={() => {const next = connectDraft(draft, draft.entry_node, selected, 'backup'); if (next) onChange(next);}}>Добавить в резерв</Button>
    </div></div>}
    <p className="muted">Все назначения доступны также в форме ниже. Выделенную основную или резервную стрелку можно удалить клавишей Delete.</p>
  </div>;
}
