export function canDismiss(dirty:boolean,busy=false,confirmDiscard=()=>window.confirm('Есть несохранённые изменения. Закрыть без сохранения?')){
 return !busy&&(!dirty||confirmDiscard());
}
