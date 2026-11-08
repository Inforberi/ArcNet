import {useEffect,useState} from 'react';
import {Button} from '../../shared/ui';

export const directAccessLabel=(link:string,index:number)=>{
  try{const parsed=new URL(link);const protocol=parsed.protocol==='hysteria2:'||parsed.protocol==='hy2:'?'Hysteria2':parsed.searchParams.get('type')==='xhttp'?'VLESS XHTTP':parsed.protocol==='vless:'?'VLESS REALITY':parsed.protocol.replace(':','').toUpperCase();return `${protocol} · ${parsed.host}`;}
  catch{return `Подключение ${index+1}`;}
};
export default function DirectAccess({links,ids,qrURL,onCopy,onError,subscription,preferredMode,preferredKind}:{links:string[];ids:string[];qrURL:(id:string)=>string;onCopy:(link:string)=>void|Promise<void>;onError:()=>void;preferredMode?:string;preferredKind?:string;subscription?:{title?:string;url:string;qrURL:string}}){
  const [selection,setSelection]=useState(()=>{const index=links.findIndex(value=>{try{const url=new URL(value);return preferredKind==='hysteria2'?['hysteria2:','hy2:'].includes(url.protocol):preferredKind?.startsWith('vless')&&url.protocol==='vless:'&&(preferredKind==='vless-xhttp'?url.searchParams.get('type')==='xhttp':url.searchParams.get('type')!=='xhttp')}catch{return false}});return ids[index>=0?index:0]??''});
  const [method,setMethod]=useState<'subscription'|'direct'>(preferredMode==='direct'?'direct':subscription?'subscription':'direct');
  useEffect(()=>{if(!ids.includes(selection))setSelection(ids[0]??'');},[ids,selection]);
  const index=ids.indexOf(selection),link=index>=0?links[index]:'';
  if(!links.length)return null;
  const subscribing=method==='subscription'&&subscription;
  const selectedLink=subscribing?subscription.url:link;
  const selectedQR=subscribing?subscription.qrURL:qrURL(selection);
  return <div className="direct-access">
    {subscription&&<div className="flex wrap" role="group" aria-label="Способ подключения"><Button aria-pressed={method==='subscription'} variant={method==='subscription'?'primary':'default'} onClick={()=>setMethod('subscription')}>Подписка</Button><Button aria-pressed={method==='direct'} variant={method==='direct'?'primary':'default'} onClick={()=>setMethod('direct')}>Одно подключение</Button></div>}
    {subscribing&&subscription.title&&<p className="muted">Название в приложении: <strong className="text-[var(--text)]">{subscription.title}</strong></p>}
    {subscribing?<p className="muted">Скопируйте ссылку и добавьте подписку в VPN-приложение. Или отсканируйте QR.</p>:<label className="field"><span>Подключение</span><select aria-label="Прямое подключение" value={selection} onChange={e=>setSelection(e.target.value)}>{links.map((value,index)=><option key={ids[index]} value={ids[index]}>{directAccessLabel(value,index)}</option>)}</select></label>}
    {selectedLink&&<><Button variant="primary" onClick={()=>void onCopy(selectedLink)}>{subscribing?'Копировать подписку':'Копировать ссылку'}</Button><img className="qr-image" key={selectedQR} src={selectedQR} alt={subscribing?'QR подписки':'QR прямого подключения'} onError={onError}/><details className="profile-advanced"><summary>Показать ссылку</summary><pre className="code">{selectedLink}</pre></details></>}
    <small className="muted">Ссылка и QR предоставляют доступ к вашему VPN.</small>
  </div>;
}
