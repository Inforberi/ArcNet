import {useState, type FormEvent, type ReactNode} from 'react';
import {useMutation, useQueryClient} from '@tanstack/react-query';
import Dialog, {canDismiss} from '../../shared/Dialog';
import {Advanced, Button, Field, Notice} from '../../shared/ui';
import {api, ApiError} from '../../shared/api';
import {quotaBytes, usageGB, usageAmount, type UsageReply, type UserPackage} from './usage';

export function UsageSummary({item,complete}:{item?:UserPackage;complete:boolean}){
 if(!item)return <span className="muted">Нет данных</span>;
 const observed=item.observed_at>0,spent=item.upload_bytes+item.download_bytes;
 return <div className="usage-summary"><strong title={observed?'Получено и отправлено вместе':'Расход ещё не измерен'}>{observed?usageAmount(spent):'—'}</strong><small className="muted">{item.quota_bytes>0?`Пакет ${usageGB(item.quota_bytes)}`:'Без лимита трафика'}</small>
 {observed&&item.quota_bytes>0&&<progress value={Math.min(spent,item.quota_bytes)} max={item.quota_bytes} aria-label={`Израсходовано ${usageAmount(spent)} из ${usageGB(item.quota_bytes)}`}/>}
 {item.quota_bytes>0&&(!complete||item.incomplete)&&<details className="usage-details"><summary>Учёт неполный</summary><small className="muted">{item.incomplete?'Часть расхода после перезапуска не подтверждена.':'Учёт охватывает не все подключения.'}</small></details>}
 </div>;
}

function localDate(seconds: number) {
 if (!seconds) return '';
 const date = new Date(seconds * 1000);
 return new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, 16);
}

export default function PackageDialog({name, initial: initialValue, status, onClose, navigation}: {name: string; initial: UserPackage; status: UsageReply; onClose: () => void; navigation?: ReactNode}) {
 const cache = useQueryClient();
 const [initial, setInitial] = useState(initialValue);
 const [unlimited, setUnlimited] = useState(initial.quota_bytes === 0);
 const [quota, setQuota] = useState(initial.quota_bytes ? String(initial.quota_bytes / 1e9) : '10');
 const [expires, setExpires] = useState(localDate(initial.expires_at));
 const [reset, setReset] = useState(false);
 const [error, setError] = useState('');
 const [message, setMessage] = useState('');
 const [conflict, setConflict] = useState(false);
 const save = useMutation({mutationFn: (input: unknown) => api<{saved: boolean; runtime_state: string}>(`/api/users/${initial.user_id}/package`, 'PUT', input)});
 async function submit(event: FormEvent<HTMLFormElement>) {
  event.preventDefault(); setError(''); setMessage(''); setConflict(false);
  const bytes = unlimited ? 0 : quotaBytes(quota);
  if (bytes === undefined) {setError('Укажите положительный пакет в ГБ, до трёх знаков после запятой.'); return;}
  const timestamp = expires ? Math.floor(new Date(expires).getTime() / 1000) : 0;
  if (!Number.isSafeInteger(timestamp) || timestamp < 0) {setError('Проверьте срок действия.'); return;}
  try {
   const reply = await save.mutateAsync({quota_bytes: bytes, expires_at: timestamp, expected_version: initial.version, reset_usage: reset});
   await cache.invalidateQueries({queryKey: ['user-usage']});
   if (reply.runtime_state === 'applied') onClose();
   else setMessage('Пакет сохранён. Применение доступа ещё не подтверждено; панель повторит попытку. Закройте окно и проверьте состояние пользователя.');
  } catch (err) {
   if (err instanceof ApiError && err.status === 409) {
    await cache.invalidateQueries({queryKey: ['user-usage']});
    const latest = cache.getQueryData<UsageReply>(['user-usage'])?.items.find(item => item.user_id === initial.user_id);
    setConflict(!!latest && latest.version !== initial.version);
   }
   setError(err instanceof Error ? err.message : 'Не удалось сохранить пакет');
  }
 }
 const unavailable = !unlimited && (!status.complete_coverage || !status.source_fresh);
 const plannedQuota = unlimited ? 0 : quotaBytes(quota);
 const plannedExpiry = expires ? Math.floor(new Date(expires).getTime() / 1000) : 0;
 const changed = plannedQuota !== initial.quota_bytes || plannedExpiry !== initial.expires_at || reset;
 const quotaLabel = (bytes: number) => bytes === 0 ? 'Без лимита' : usageGB(bytes);
 const expiryLabel = (seconds: number) => seconds ? new Date(seconds * 1000).toLocaleString('ru-RU') : 'Без срока';
 return <Dialog title={`Пакет · ${name}`} onClose={onClose} busy={save.isPending} dirty={changed && !message}>
  {navigation&&<div onClickCapture={event=>{if((event.target as HTMLElement).closest('button[aria-pressed="false"]')&&!canDismiss(changed && !message,save.isPending)){event.preventDefault();event.stopPropagation();}}}>{navigation}</div>}
  <form className="dialog-body" onSubmit={event => void submit(event)}>
   <UsageSummary item={initialValue} complete={status.complete_coverage}/>
   <label className="flex"><input type="checkbox" checked={unlimited} onChange={event => setUnlimited(event.target.checked)}/><span>Без ограничения трафика</span></label>
   {!unlimited && <Field label="Пакет, ГБ" aria-label="Пакет, ГБ" inputMode="decimal" required value={quota} onChange={event => setQuota(event.target.value)} description="1 ГБ = 1 000 000 000 байт. Считаем загрузку и отправку вместе."/>}
   {unavailable && <Notice>Для конечного пакета нужны свежие счётчики и учёт всех выданных подключений. Поддержаны локальные Xray, Hysteria2 и WireGuard; AmneziaWG и ноды не входят в поддержанный учёт.</Notice>}
   <Field label="Действует до" aria-label="Действует до" type="datetime-local" value={expires} onChange={event => setExpires(event.target.value)} description="Пусто — без срока. Время в часовом поясе вашего браузера."/>
   <Advanced title="Новый период пакета">
    <label className="flex"><input type="checkbox" checked={reset} onChange={event => setReset(event.target.checked)}/><span>Начать новый период и обнулить расход пакета</span></label>
    <small>Перед новым периодом панель получает свежие счётчики. При ошибке расход не сбрасывается. Ключи и ссылки пользователя сохраняются.</small>
   </Advanced>
   {changed && plannedQuota !== undefined && Number.isFinite(plannedExpiry) && <div className="notice" aria-label="План изменения пакета">
    <strong>План изменения</strong>
    {plannedQuota !== initial.quota_bytes && <div>Трафик: {quotaLabel(initial.quota_bytes)} → {quotaLabel(plannedQuota)}</div>}
    {plannedExpiry !== initial.expires_at && <div>Срок: {expiryLabel(initial.expires_at)} → {expiryLabel(plannedExpiry)}</div>}
    {reset && <div>Новый период: расход начнётся с нуля, учёт прежнего периода сохранится.</div>}
   </div>}
   {error && <p className="error" role="alert">{error}</p>}
   {conflict && <div><Button type="button" onClick={() => {setInitial(initialValue); setConflict(false); setError('');}}>Обновить версию</Button><p className="muted">Ваши поля сохранятся. Проверьте новый план перед сохранением.</p></div>}
   {message && <Notice>{message}</Notice>}
   <div className="form-actions"><Button type="button" disabled={save.isPending} onClick={()=>{if(canDismiss(changed && !message,save.isPending))onClose();}}>Закрыть</Button><Button type="submit" variant="primary" disabled={save.isPending || unavailable || !!message}>Сохранить пакет</Button></div>
  </form>
 </Dialog>;
}
