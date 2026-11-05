'use strict';
// Theme is an unprivileged presentation preference. Authentication cookies remain HttpOnly.
(function(){
  function get(){
    const item=document.cookie.split('; ').find(p=>p.startsWith('arcvpn_theme='));
    const name=item&&item.slice('arcvpn_theme='.length);
    return name==='light'?'light':'dark';
  }
  function set(name){
    const mode=name==='light'?'light':'dark';
    document.documentElement.dataset.theme=mode;
    document.cookie='arcvpn_theme='+mode+'; Path=/; Max-Age=31536000; SameSite=Lax'+(location.protocol==='https:'?'; Secure':'');
    return mode;
  }
  document.documentElement.dataset.theme=get();
  window.ArcVPNTheme={get,set,toggle:()=>set(get()==='dark'?'light':'dark')};
})();
