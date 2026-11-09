import {useEffect,useRef,useState} from 'react';
import {Button} from '../../shared/ui';

export const directAccessLabel=(link:string,index:number)=>{
  try{const parsed=new URL(link);const protocol=parsed.protocol==='hysteria2:'||parsed.protocol==='hy2:'?'Hysteria2':parsed.searchParams.get('type')==='xhttp'?'VLESS XHTTP':parsed.protocol==='vless:'?'VLESS REALITY':parsed.protocol.replace(':','').toUpperCase();return `${protocol} · ${parsed.host}`;}
  catch{return `Подключение ${index+1}`;}
};
export default function DirectAccess({links,ids,qrURL,onCopy,onError,subscription,preferredMode,preferredKind}:{links:string[];ids:string[];qrURL:(id:string)=>string;onCopy:(link:string)=>void|Promise<void>;onError:()=>void;preferredMode?:string;preferredKind?:string;subscription?:{title?:string;url:string;qrURL:string}}){
  const [qrLoaded,setQRLoaded]=useState(''),[qrFailed,setQRFailed]=useState('');
  const [copyResult,setCopyResult]=useState<{key:string;state:'busy'|'copied'|'failed'}>();
  const copying=useRef(false);
  const [selection,setSelection]=useState(()=>{const index=links.findIndex(value=>{try{const url=new URL(value);return preferredKind==='hysteria2'?['hysteria2:','hy2:'].includes(url.protocol):preferredKind?.startsWith('vless')&&url.protocol==='vless:'&&(preferredKind==='vless-xhttp'?url.searchParams.get('type')==='xhttp':url.searchParams.get('type')!=='xhttp')}catch{return false}});return ids[index>=0?index:0]??''});
  const [method,setMethod]=useState<'subscription'|'direct'>(preferredMode==='direct'?'direct':subscription?'subscription':'direct');
  useEffect(()=>{if(!ids.includes(selection))setSelection(ids[0]??'');},[ids,selection]);
  const index=ids.indexOf(selection),link=index>=0?links[index]:'';
  useEffect(()=>{if(copyResult?.state!=='copied')return;const timer=setTimeout(()=>setCopyResult(undefined),4500);return ()=>clearTimeout(timer)},[copyResult]);
  if(!links.length&&!subscription)return null;
  const subscribing=subscription&&(method==='subscription'||!links.length);
  const selectedLink=subscribing?subscription.url:link;
  const selectedQR=subscribing?subscription.qrURL:qrURL(selection);
  async function copy(){if(copying.current)return;copying.current=true;setCopyResult({key:selectedQR,state:'busy'});try{await onCopy(selectedLink);setCopyResult({key:selectedQR,state:'copied'})}catch{setCopyResult({key:selectedQR,state:'failed'})}finally{copying.current=false}}
  return <div className="direct-access">
    {subscription&&<div className="access-methods" role="group" aria-label="Способ подключения"><Button aria-pressed={!!subscribing} onClick={()=>setMethod('subscription')}>Подписка</Button><Button aria-pressed={!subscribing} disabled={!links.length} onClick={()=>setMethod('direct')}>Одно подключение</Button></div>}
    <div className="access-profile-slot">{subscribing?<div className="field"><span>Название в приложении</span><strong className="access-profile-value">{subscription.title||'ArcNet'}</strong></div>:<label className="field"><span>Подключение</span><select aria-label="Прямое подключение" value={selection} onChange={e=>setSelection(e.target.value)}>{links.map((value,index)=><option key={ids[index]} value={ids[index]}>{directAccessLabel(value,index)}</option>)}</select></label>}</div>
    {selectedLink&&<div className="access-code-layout"><figure className="access-code"><div className="qr-frame" aria-busy={qrLoaded!==selectedQR}>{qrFailed===selectedQR?<span className="qr-failed" role="status">Не удалось загрузить QR.<br/>Скопируйте ссылку ниже.</span>:qrLoaded!==selectedQR&&<span className="qr-loading" role="status">Загружаем QR…</span>}<img className="qr-image" key={selectedQR} src={selectedQR} width={224} height={224} alt={subscribing?'QR подписки':'QR прямого подключения'} onLoad={()=>{setQRLoaded(selectedQR);setQRFailed('')}} onError={()=>{setQRLoaded(selectedQR);setQRFailed(selectedQR);onError()}}/></div><figcaption>Сканируйте камерой VPN-приложения</figcaption></figure><div className="access-code-actions"><strong>{subscribing?'Одна ссылка для всех подключений':'Прямое подключение'}</strong><p className="muted">{subscribing?'Добавьте ссылку как подписку. Новые параметры появятся после её обновления в приложении.':'Импортируйте ссылку в VPN-приложение или отсканируйте код с другого устройства.'}</p><Button variant="primary" pending={copyResult?.state==='busy'} onClick={()=>void copy()}>{subscribing?'Копировать подписку':'Копировать ссылку'}</Button><div className="copy-result" aria-live="polite">{copyResult?.key===selectedQR&&copyResult.state==='copied'&&'Ссылка скопирована'}{copyResult?.key===selectedQR&&copyResult.state==='failed'&&<span className="error">Не удалось скопировать. Откройте «Показать ссылку» и скопируйте вручную.</span>}</div><details className="profile-advanced"><summary>Показать ссылку</summary><pre className="code">{selectedLink}</pre></details></div></div>}
    <small className="muted">Ссылка и QR предоставляют доступ к вашему VPN.</small>
  </div>;
}
