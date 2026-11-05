import {useEffect, useRef, useState} from 'react';
import {useQuery, useQueryClient} from '@tanstack/react-query';
import {api} from '../../shared/api';
import {Badge, Button, Field, Loading} from '../../shared/ui';
import {candidateKey, usableResult, type Candidate, type TargetJob} from './model';

type CatalogReply = {catalog: {version: number; candidates: Candidate[]; source: string}; signed_update_enabled: boolean};
type Node = {id: string; name: string; supported: boolean; online: boolean};
const message = (error: unknown) => error instanceof Error ? error.message : 'Не удалось выполнить проверку';

export default function TargetPicker({nodeID = 'local', sameName = false, onSelect, onRecommend}: {nodeID?: string; sameName?: boolean; onSelect?: (candidate: Candidate) => void; onRecommend?: (candidate: Candidate) => void}) {
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
  const candidates = catalog.data?.catalog.candidates ?? [];
  const keys = selected ?? candidates.slice(0, 4).map(candidateKey);
  const node = nodes.data?.items.find(item => item.id === nodeID);
  const running = !!job && ['queued', 'running'].includes(job.state) && job.expires_at > now;
  const pending = useQuery({queryKey: ['target-job', job?.id], queryFn: () => api<TargetJob>(`/api/targets/checks/${encodeURIComponent(job!.id)}`), enabled: running, refetchInterval: running ? 2000 : false});
  useEffect(() => {if (!job) return; const timer = window.setInterval(() => setNow(Date.now() / 1000), 1000); return () => window.clearInterval(timer);}, [job?.id]);
  useEffect(() => {setJob(undefined); setError('');}, [nodeID]);
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
    <p className="muted">Выберите до четырёх кандидатов. Проверим сертификат, TLS 1.3, HTTP/2 и три замера TCP+TLS с выбранного сервера.</p>
    {catalog.isPending ? <Loading/> : catalog.isError ? <p role="alert" className="error">{message(catalog.error)}</p> : <>
      <div className="target-candidates">{candidates.map(candidate => <label key={candidateKey(candidate)}><input type="checkbox" checked={keys.includes(candidateKey(candidate))} disabled={busy || running || (!keys.includes(candidateKey(candidate)) && keys.length >= 4)} onChange={event => setSelected(event.target.checked ? [...keys, candidateKey(candidate)] : keys.filter(key => key !== candidateKey(candidate)))}/><span>{candidate.host}{candidate.sni !== candidate.host && <small>SNI: {candidate.sni}</small>}</span></label>)}</div>
      <Button type="button" disabled={busy || running || !keys.length || !node?.online || !node.supported} onClick={() => void check()}>{busy || running ? 'Проверяем…' : 'Проверить и подобрать'}</Button>
      {node && (!node.online || !node.supported) && <p className="muted">Нужен доступный агент с поддержкой SNI-проверок.</p>}
      {nodes.isError && <p className="error" role="alert">{message(nodes.error)}</p>}
      {job && <div aria-live="polite">
        {running && <p>Ожидаем результат сервера. Проверка ограничена по времени.</p>}
        {!running && job.state !== 'done' && <p>Сервер не завершил проверку вовремя. Результат не применяется.</p>}
        {job.report && <><p className="muted">{job.report.expires_at > now ? 'Результаты действительны 10 минут.' : 'Результаты устарели — повторите проверку.'} Это проверка target, без подтверждения VPN-подключения.</p>
          <div className="target-results">{job.report.results.map(result => <div className="target-result" key={candidateKey(result.candidate)}><div><strong>{result.candidate.host}</strong><small>{result.eligible ? `${result.successes}/3 · медиана ${result.median_ms.toFixed(1)} мс · максимум ${result.p95_ms.toFixed(1)} мс` : result.error ?? 'Цель не прошла проверку'}{sameName && result.candidate.host !== result.candidate.sni ? ' · Для основного профиля нужны одинаковые target и SNI.' : ''}</small></div>{onSelect && <Button type="button" disabled={!usableResult(job, nodeID, result.candidate, now) || (sameName && result.candidate.host !== result.candidate.sni)} onClick={() => {if (usableResult(job, nodeID, result.candidate) && (!sameName || result.candidate.host === result.candidate.sni)) onSelect(result.candidate);}}>Выбрать</Button>}</div>)}</div></>}
      </div>}
      {pending.isError && running && <p className="error" role="alert">{message(pending.error)}</p>}
      <details className="profile-advanced"><summary>Свой список SNI</summary><div className="profile-settings"><label className="field"><span>Кандидаты, по одному на строке</span><textarea rows={4} maxLength={65536} value={text} disabled={busy || running || !!url.trim()} onChange={event => setText(event.target.value)} placeholder={'example.com\ntarget.example.com,sni.example.com'}/></label><Field label="Файл списка" type="file" accept="text/plain,.txt" disabled={busy || running || !!url.trim()} onChange={async event => {const file = event.target.files?.[0]; event.target.value = ''; if (!file) return; if (file.size > 65536) {setError('Размер списка — не более 64 КБ'); return;} try {setText(await file.text());} catch {setError('Не удалось прочитать файл');}}}/>{catalog.data?.signed_update_enabled ? <Field label="HTTPS URL подписанного каталога (вместо текста)" value={url} disabled={busy || running} onChange={event => setURL(event.target.value)} /> : <p className="muted">HTTPS-обновление каталога доступно после установки независимого ключа подписи администратором.</p>}<Button type="button" disabled={busy || running || (!text.trim() && !url.trim())} onClick={() => void importCatalog()}>Заменить каталог кандидатов</Button><small>Каталог общий для панели. Импорт не меняет действующие SNI; каждый кандидат требует отдельной проверки.</small></div></details>
    </>}
    {error && <p className="error" role="alert">{error}</p>}
  </div>;
}
