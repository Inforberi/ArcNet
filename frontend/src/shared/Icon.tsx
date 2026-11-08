import type {ReactNode} from 'react';

const shapes={
 plus:<path d="M12 4v16M4 12h16"/>,
 edit:<><path d="m15 4 5 5M4 20l4-1 12-12a2 2 0 0 0-5-5L3 14l-1 6Z"/></>,
 refresh:<><path d="M20 7v5h-5M4 17v-5h5"/><path d="M6 7a7 7 0 0 1 12-1l2 3M4 15l2 3a7 7 0 0 0 12-1"/></>,
 shield:<><path d="m12 3 8 3v6c0 4-4 7-8 9-4-2-8-5-8-9V6Z"/><path d="m8 12 3 3 5-6"/></>,
 wave:<path d="M2 12h4l3-8 5 16 3-8h5"/>,
 qr:<><rect x="3" y="3" width="6" height="6"/><rect x="15" y="3" width="6" height="6"/><rect x="3" y="15" width="6" height="6"/><path d="M15 15h3v3h3v3h-6v-3m6-3h.01"/></>,
 trash:<><path d="M3 6h18M9 6V3h6v3M5 6l1 15h12l1-15M10 10v7m4-7v7"/></>,
 info:<><circle cx="12" cy="12" r="9"/><path d="M12 11v6m0-10h.01"/></>,
 download:<><path d="M12 3v12m-4-4 4 4 4-4M4 17v4h16v-4"/></>,
 overview:<><rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/></>,
 protocols:<><rect x="8" y="3" width="8" height="6" rx="1.5"/><path d="M12 9v5M5 14h14M5 14v3m14-3v3"/><rect x="2" y="17" width="6" height="4" rx="1"/><rect x="16" y="17" width="6" height="4" rx="1"/></>,
 users:<><circle cx="9" cy="8" r="3"/><path d="M3 21v-2a6 6 0 0 1 12 0v2m1-17a3 3 0 0 1 0 6m2 5a5 5 0 0 1 3 4v2"/></>,
 servers:<><rect x="3" y="3" width="18" height="7" rx="2"/><rect x="3" y="14" width="18" height="7" rx="2"/><path d="M7 6.5h.01M7 17.5h.01M11 6.5h6m-6 11h6"/></>,
 routes:<><circle cx="5" cy="5" r="2"/><circle cx="19" cy="19" r="2"/><path d="M7 5h9a4 4 0 0 1 0 8H8a3 3 0 0 0 0 6h9"/></>,
 settings:<><path d="M4 7h16M4 17h16"/><rect x="7" y="4" width="4" height="6" rx="1"/><rect x="14" y="14" width="4" height="6" rx="1"/></>,
 logs:<><rect x="4" y="3" width="16" height="18" rx="2"/><path d="M8 8h8M8 12h8M8 16h5"/></>,
 backup:<><path d="M4 7v13h16V7M3 3h18v4H3zM9 11h6"/></>,
 sun:<><circle cx="12" cy="12" r="4"/><path d="M12 2v2m0 16v2M2 12h2m16 0h2M5 5l1.5 1.5m11 11L19 19M5 19l1.5-1.5m11-11L19 5"/></>,
 moon:<path d="M21 13a9 9 0 1 1-10-10 7 7 0 0 0 10 10Z"/>,
 logout:<><path d="M9 4H4v16h5m5-13 5 5-5 5m-5-5h10"/></>,
 menu:<path d="M4 6h16M4 12h16M4 18h16"/>,
 close:<path d="m6 6 12 12M6 18 18 6"/>,
 search:<><circle cx="10.5" cy="10.5" r="6.5"/><path d="m16 16 5 5"/></>,
 chevron:<path d="m6 9 6 6 6-6"/>,
} satisfies Record<string,ReactNode>;
export type IconName=keyof typeof shapes;
export function Icon({name,className=''}:{name:IconName;className?:string}){
 return <svg className={`h-5 w-5 shrink-0 ${className}`} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false">{shapes[name]}</svg>;
}
