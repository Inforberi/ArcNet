export default function FingerprintField({value='chrome',onChange,disabled=false}:{value?:string;onChange:(value:string)=>void;disabled?:boolean}) {
 return <label className="field"><span>TLS fingerprint клиента</span><select aria-label="TLS fingerprint клиента" value={value||'chrome'} disabled={disabled} onChange={event=>onChange(event.target.value)}>
  {[['chrome','Chrome · по умолчанию'],['firefox','Firefox'],['safari','Safari'],['ios','iOS'],['android','Android'],['edge','Edge'],['360','360 Browser'],['qq','QQ Browser'],['random','Случайный браузерный профиль'],['randomized','Случайно сформированный TLS-профиль']].map(([id,label])=><option key={id} value={id}>{label}</option>)}
 </select><small>Профиль TLS для VPN-клиента. Он не меняет браузер на устройстве. Поддержка зависит от клиента; после изменения обновите подписку.</small></label>;
}
