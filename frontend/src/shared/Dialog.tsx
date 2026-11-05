import {useEffect, useRef, type ReactNode} from 'react';
import {Button} from './ui';

export default function Dialog({title, children, onClose, busy = false}: {title: string; children: ReactNode; onClose: () => void; busy?: boolean}) {
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => {const dialog = ref.current; dialog?.showModal(); return () => {dialog?.close();};}, []);
  return <dialog ref={ref} className="app-dialog" aria-label={title} onCancel={event => {event.preventDefault(); if (!busy) onClose();}}>
    <header className="dialog-heading"><h2>{title}</h2><Button type="button" variant="ghost" aria-label={`Закрыть: ${title}`} disabled={busy} onClick={onClose}>✕</Button></header>
    {children}
  </dialog>;
}
