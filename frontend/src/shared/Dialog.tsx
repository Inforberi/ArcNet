import {useEffect, useRef, useState, type ReactNode} from 'react';
import {Button} from './ui';
import {Icon} from './Icon';

export function canDismiss(dirty: boolean, busy = false) {
  return !busy && (!dirty || window.confirm('Есть несохранённые изменения. Закрыть без сохранения?'));
}

export default function Dialog({title, children, onClose, busy = false, dirty, className = ''}: {title: string; children: ReactNode; onClose: () => void; busy?: boolean; dirty?: boolean; className?: string}) {
  const ref = useRef<HTMLDialogElement>(null);
  const [edited, setEdited] = useState(false);
  const backdrop = useRef(false);
  const dismiss = () => {if (canDismiss(dirty ?? edited, busy)) onClose();};
  function outside(x: number, y: number) {
    const box = ref.current?.getBoundingClientRect();
    return !!box && (x < box.left || x > box.right || y < box.top || y > box.bottom);
  }
  useEffect(() => {const dialog = ref.current; dialog?.showModal(); return () => {dialog?.close();};}, []);
  return <dialog ref={ref} className={`app-dialog m-auto ${className}`} aria-label={title}
    onInputCapture={event => {if ((event.target as HTMLElement).closest('form')) setEdited(true);}}
    onPointerDown={event => {backdrop.current = event.target === event.currentTarget && outside(event.clientX, event.clientY);}}
    onPointerCancel={() => {backdrop.current = false;}}
    onClick={event => {const clickedOutside = backdrop.current; backdrop.current = false; if (clickedOutside && event.target === event.currentTarget && outside(event.clientX, event.clientY)) dismiss();}}
    onCancel={event => {event.preventDefault(); dismiss();}}>
    <header className="dialog-heading"><h2>{title}</h2><Button type="button" variant="ghost" aria-label={`Закрыть: ${title}`} disabled={busy} onClick={dismiss}><Icon name="close"/></Button></header>
    {children}
  </dialog>;
}
