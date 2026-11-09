import {useId,useRef} from 'react';

type Action={label:string;onClick:()=>void;disabled?:boolean;reason?:string;danger?:boolean};
// Native popover escapes table overflow without a portal or another dependency.
export default function ActionMenu({label,items,disabled=false}:{label:string;items:Action[];disabled?:boolean}){
 const id=useId(),panel=useRef<HTMLDivElement>(null),anchor=useRef<HTMLButtonElement>(null);
 function close(){panel.current?.hidePopover();anchor.current?.focus()}
 return <><button ref={anchor} type="button" className="button ghost action-menu-trigger" aria-label={label} title={label} popoverTarget={id} disabled={disabled}>•••</button>
 <div ref={panel} id={id} popover="auto" className="action-menu" role="group" aria-label={label} onToggle={e=>{
  if(e.newState!=='open'||!anchor.current||!panel.current)return;
  const box=anchor.current.getBoundingClientRect(),menu=panel.current;
  menu.style.left=Math.max(12,Math.min(box.right-menu.offsetWidth,innerWidth-menu.offsetWidth-12))+'px';
  menu.style.top=Math.max(12,Math.min(box.bottom+6,innerHeight-menu.offsetHeight-12))+'px';
  menu.querySelector<HTMLButtonElement>('button:not(:disabled)')?.focus();
 }} onKeyDown={e=>{if(e.key==='Escape'){e.preventDefault();e.stopPropagation();close()}else if(e.key==='ArrowDown'||e.key==='ArrowUp'){e.preventDefault();const buttons=[...e.currentTarget.querySelectorAll<HTMLButtonElement>('button:not(:disabled)')],index=buttons.indexOf(document.activeElement as HTMLButtonElement);buttons[(index+(e.key==='ArrowDown'?1:-1)+buttons.length)%buttons.length]?.focus()}}}>
 {items.map(item=><div key={item.label}><button type="button" className={`button ghost ${item.danger?'danger':''}`} disabled={item.disabled} onClick={()=>{close();item.onClick()}}>{item.label}</button>{item.reason&&<small className="muted">{item.reason}</small>}</div>)}
 </div></>;
}
