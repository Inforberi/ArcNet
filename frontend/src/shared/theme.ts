export type Theme='dark'|'light';
export function readTheme():Theme {
  const item=document.cookie.split('; ').find(pair=>pair.startsWith('arcvpn_theme='));
  return item?.slice('arcvpn_theme='.length)==='light'?'light':'dark';
}
export function applyTheme(mode:Theme):void {
  document.documentElement.dataset.theme=mode;
  document.cookie=`arcvpn_theme=${mode}; Path=/; Max-Age=31536000; SameSite=Lax${location.protocol==='https:'?'; Secure':''}`;
}
export function restoreTheme():void {document.documentElement.dataset.theme=readTheme()}
