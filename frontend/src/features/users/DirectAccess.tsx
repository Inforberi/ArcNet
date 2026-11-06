import {useEffect, useState} from 'react';
import {Button, Notice} from '../../shared/ui';

export const directAccessLabel = (link: string, index: number) => {
  try {const parsed = new URL(link); const protocol = parsed.protocol === 'hysteria2:' || parsed.protocol === 'hy2:' ? 'Hysteria2' : parsed.searchParams.get('type') === 'xhttp' ? 'VLESS XHTTP' : parsed.protocol === 'vless:' ? 'VLESS REALITY' : parsed.protocol.replace(':', '').toUpperCase(); return `${protocol} · ${parsed.host}`;}
  catch {return `Подключение ${index + 1}`;}
};
export default function DirectAccess({links, ids, qrURL, onCopy, onError}: {links: string[]; ids: string[]; qrURL: (id: string) => string; onCopy: (link: string) => void | Promise<void>; onError: () => void}) {
  const [selection, setSelection] = useState(ids[0] ?? '');
  useEffect(() => {if (!ids.includes(selection)) setSelection(ids[0] ?? '');}, [ids, selection]);
  const index = ids.indexOf(selection);
  const link = index >= 0 ? links[index] : '';
  if (!links.length) return <Notice>Для WireGuard используйте файл .conf ниже. Прямых URI-подключений пока нет.</Notice>;
  return <div className="direct-access">
    <label className="field"><span>Прямое подключение</span><select aria-label="Прямое подключение" value={selection} onChange={e => setSelection(e.target.value)}>{links.map((value, index) => <option key={ids[index]} value={ids[index]}>{directAccessLabel(value, index)}</option>)}</select></label>
    <p className="muted">Для телевизора или ручного импорта используйте одну ссылку либо QR. Подписка не требуется. Ссылка содержит ключ доступа.</p>
    {link && <><pre className="code">{link}</pre><Button onClick={() => void onCopy(link)}>Копировать прямую ссылку</Button><img className="qr-image" key={selection} src={qrURL(selection)} alt="QR прямого подключения" onError={onError}/></>}
  </div>;
}
