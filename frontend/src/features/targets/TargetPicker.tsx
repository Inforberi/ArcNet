import {useEffect, useRef, useState} from 'react';
import {useQuery, useQueryClient} from '@tanstack/react-query';
import {api} from '../../shared/api';
import {Badge, Advanced, Button, Field, Loading} from '../../shared/ui';
import {candidateKey, usableResult, type Candidate, type TargetJob, type RecommendationState} from './model';

type CatalogReply = {catalog: {version: number; candidates: Candidate[]; source: string}; signed_update_enabled: boolean};
type Node = {id: string; name: string; supported: boolean; online: boolean};
const message = (error: unknown) => error instanceof Error ? error.message : 'Не удалось выполнить проверку';

export default function TargetPicker({nodeID = 'local', sameName = false, automatic = false, onSelect, onRecommend, onStatus}: {nodeID?: string; sameName?: boolean; automatic?: boolean; onSelect?: (candidate: Candidate) => void; onRecommend?: (candidate: Candidate) => void; onStatus?: (state: RecommendationState) => void}) {
  const cache = useQueryClient();
  const catalog = useQuery({queryKey: ['target-catalog'], queryFn: () => api<CatalogReply>('/api/targets/catalog'), staleTime: 60000});
  const nodes = useQuery({queryKey: ['target-nodes'], queryFn: () => api<{items: Node[]}>('/api/targets/nodes'), staleTime: 15000});
  const [selected, setSelected] = useState<string[] | null>(null);
  const [job, setJob] = useState<TargetJob>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [text, setText] = useState('');
  const [url, setURL] = useState('');
  const [now, setNow] = useState(Date.now() / 1000);
  const latestNode = useRef(nodeID); latestNode.current = nodeID;
  const recommendation = useRef(onRecommend); recommendation.current = onRecommend;
  const statusCallback = useRef(onStatus); statusCallback.current = onStatus;
  const candidates = catalog.data?.catalog.candidates ?? [];
  const automaticCandidates=candidates.filter(candidate=>!sameName||candidate.host===candidate.sni).slice(0,4);
  const keys = selected ?? candidates.slice(0, 4).map(candidateKey);
  const node = nodes.data?.items.find(item => item.id === nodeID);
  const running = !!job && ['queued', 'running'].includes(job.state) && job.expires_at > now;
  const pending = useQuery({queryKey: ['target-job', job?.id], queryFn: () => api<TargetJob>(`/api/targets/checks/${encodeURIComponent(job!.id)}`), enabled: running, refetchInterval: running ? 2000 : false});
  useEffect(() => {setJob(undefined); setError('');}, [nodeID]);
  const auto = useQuery({
    queryKey: ['target-auto-check', nodeID, catalog.data?.catalog.version, automaticCandidates.map(candidateKey)],
    queryFn: () => api<TargetJob>('/api/targets/check', 'POST', {node_id: nodeID, candidates: automaticCandidates}),
    enabled: automatic && !!catalog.data && candidates.some(candidate => !sameName || candidate.host === candidate.sni) && !!node?.online && !!node.supported,
    staleTime: 600000, gcTime: 600000, retry: false, refetchOnWindowFocus: false,
  });
  const report = job ?? auto.data;
  const best = report?.report?.recommended;
  const state: RecommendationState = catalog.isPending || nodes.isPending || auto.isFetching || running ? 'checking'
    : best && usableResult(report, nodeID, best) && (!sameName || best.host === best.sni) ? 'ready'
    : catalog.isError || nodes.isError || (!job && auto.isError) ? 'failed'
    : !node?.online || !node.supported ? 'unavailable'
    : 'failed';
  useEffect(() => {if (automatic) statusCallback.current?.(state);}, [automatic, state]);
  useEffect(() => {
    if (!automatic || !auto.data || auto.data.node_id !== nodeID) return;
    setJob(current => current ?? auto.data);
    const best = auto.data.report?.recommended;
    if (best && usableResult(auto.data, nodeID, best) && (!sameName || best.host === best.sni)) recommendation.current?.(best);
  }, [auto.data, automatic, nodeID, sameName]);
  useEffect(() => {if (!job) return; const timer = window.setInterval(() => setNow(Date.now() / 1000), 1000); return () => window.clearInterval(timer);}, [job?.id]);
  useEffect(() => {
    const result = pending.data;
    if (result && result.id === job?.id && result.node_id === nodeID) {
      setJob(result);
      const best = result.report?.recommended;
      if (best && usableResult(result, nodeID, best) && (!sameName || best.host === best.sni)) recommendation.current?.(best);
    }
  }, [pending.data]);

  async function check() {
    setBusy(true); setError(''); setJob(undefined);
    const requestedNode = nodeID;
    try {
      const result = await api<TargetJob>('/api/targets/check', 'POST', {node_id: requestedNode, candidates: candidates.filter(candidate => keys.includes(candidateKey(candidate)))});
      if (latestNode.current !== requestedNode) return;
      setJob(result); setNow(Date.now() / 1000);
      const best = result.report?.recommended;
      if (best && usableResult(result, requestedNode, best) && (!sameName || best.host === best.sni)) recommendation.current?.(best);
    } catch (error) {if (latestNode.current === requestedNode) setError(message(error));} finally {setBusy(false);}
  }
  async function importCatalog() {
    if (!catalog.data) return;
    setBusy(true); setError('');
    try {
      await api('/api/targets/catalog', 'POST', {text: url.trim() ? '' : text, url: url.trim(), expected_version: catalog.data.catalog.version});
      setSelected(null); setJob(undefined); setText(''); setURL('');
      await cache.invalidateQueries({queryKey: ['target-catalog']});
    } catch (error) {setError(message(error));} finally {setBusy(false);}
  }
  return <div className="target-picker">
    <div className="flex wrap"><strong>Подбор Reality SNI</strong><Badge>{node?.name ?? nodeID}</Badge></div>
    <p className="muted">Выберите до четырёх доменов для проверки с этого сервера. Каталог содержит {candidates.length} кандидатов; можно добавить свой список.</p>
    {catalog.isPending ? <Loading/> : catalog.isError ? <p role="alert" className="error">{message(catalog.error)}</p> : <>
      <div className="target-candidates">{candidates.map(candidate => <label key={candidateKey(candidate)}><input type="checkbox" checked={keys.includes(candidateKey(candidate))} disabled={busy || running || (!keys.includes(candidateKey(candidate)) && keys.length >= 4)} onChange={event => setSelected(event.target.checked ? [...keys, candidateKey(candidate)] : keys.filter(key => key !== candidateKey(candidate)))}/><span>{candidate.host}{candidate.sni !== candidate.host && <small>SNI: {candidate.sni}</small>}</span></label>)}</div>
      <Button type="button" disabled={busy || running || auto.isFetching || !keys.length || !node?.online || !node.supported} onClick={() => void check()}>{busy || running || auto.isFetching ? 'Проверяем…' : 'Проверить и подобрать'}</Button>
      {!job&&!busy&&!error&&auto.isError && <p className="error" role="alert">{message(auto.error)} Можно указать SNI вручную.</p>}
      {node && (!node.online || !node.supported) && <p className="muted">Нужен доступный агент с поддержкой SNI-проверок.</p>}
      {nodes.isError && <p className="error" role="alert">{message(nodes.error)}</p>}
      {job && <div aria-live="polite">
        {running && <p>Ожидаем результат сервера. Проверка ограничена по времени.</p>}
        {!running && job.state !== 'done' && <p>Сервер не завершил проверку вовремя. Результат не применяется.</p>}
        {job.report && <>{job.report.results.length>0 && job.report.results.every(result=>!result.eligible) && <p role="status" className="muted">Ни один домен не прошёл проверку с этого сервера. Текущие настройки сохранены. Повторите проверку или укажите свой домен; причины показаны ниже.</p>}<p className="muted">{job.report.expires_at > now ? 'Результаты действительны 10 минут.' : 'Результаты устарели — повторите проверку.'} Это проверка target, без подтверждения VPN-подключения.</p>
          <div className="target-results">{job.report.results.map(result => <div className="target-result" key={candidateKey(result.candidate)}><div><strong>{result.candidate.host}</strong><small>{result.eligible ? `${result.successes}/3 · медиана ${result.median_ms.toFixed(1)} мс · максимум ${result.p95_ms.toFixed(1)} мс` : result.error ?? 'Цель не прошла проверку'}{sameName && result.candidate.host !== result.candidate.sni ? ' · Для основного профиля нужны одинаковые target и SNI.' : ''}</small></div>{onSelect && usableResult(job, nodeID, result.candidate, now) && (!sameName || result.candidate.host === result.candidate.sni) && <Button type="button" onClick={() => {if (usableResult(job, nodeID, result.candidate) && (!sameName || result.candidate.host === result.candidate.sni)) onSelect(result.candidate);}}>Выбрать</Button>}</div>)}</div></>}
      </div>}
      {pending.isError && running && <p className="error" role="alert">{message(pending.error)}</p>}
      <Advanced title="Свой список SNI"><label className="field"><span>Кандидаты, по одному на строке</span><textarea rows={4} maxLength={65536} value={text} disabled={busy || running || !!url.trim()} onChange={event => setText(event.target.value)} placeholder={'example.com\ntarget.example.com,sni.example.com'}/></label><Field label="Файл списка" type="file" accept="text/plain,.txt" disabled={busy || running || !!url.trim()} onChange={async event => {const file = event.target.files?.[0]; event.target.value = ''; if (!file) return; if (file.size > 65536) {setError('Размер списка — не более 64 КБ'); return;} try {setText(await file.text());} catch {setError('Не удалось прочитать файл');}}}/>{catalog.data?.signed_update_enabled ? <Field label="HTTPS URL подписанного каталога (вместо текста)" value={url} disabled={busy || running} onChange={event => setURL(event.target.value)} /> : <p className="muted">HTTPS-обновление каталога доступно после установки независимого ключа подписи администратором.</p>}<Button type="button" disabled={busy || running || (!text.trim() && !url.trim())} onClick={() => void importCatalog()}>Заменить каталог кандидатов</Button><small>Каталог общий для панели. Импорт не меняет действующие SNI; каждый кандидат требует отдельной проверки.</small></Advanced>
    </>}
    {error && <p className="error" role="alert">{error}</p>}
  </div>;
}
