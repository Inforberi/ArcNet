import {useEffect, useRef, type ReactNode} from 'react';
import {Button} from './ui';
import {Icon} from './Icon';

export default function Dialog({title, children, onClose, busy = false, className = ''}: {title: string; children: ReactNode; onClose: () => void; busy?: boolean; className?: string}) {
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => {const dialog = ref.current; dialog?.showModal(); return () => {dialog?.close();};}, []);
  return <dialog ref={ref} className={`app-dialog m-auto ${className}`} aria-label={title} onCancel={event => {event.preventDefault(); if (!busy) onClose();}}>
    <header className="dialog-heading"><h2>{title}</h2><Button type="button" variant="ghost" aria-label={`Закрыть: ${title}`} disabled={busy} onClick={onClose}><Icon name="close"/></Button></header>
    {children}
  </dialog>;
}
