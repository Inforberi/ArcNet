#!/usr/bin/env bash
set -Eeuo pipefail
if [[ $EUID -ne 0 ]];then exec sudo -- "$0" "$@";fi

run_core(){ runuser -u arcvpn -- env ARCVPN_DATA_DIR=/var/lib/arcvpn /opt/arcvpn/bin/arcvpn-core "$@"; }
run_lifecycle(){
 local capabilities
 # help is handled before opening SQLite, including by previous cores. Never
 # send a new privileged command to an old core which might treat it as a
 # normal controller command and open the database as root.
 capabilities="$(/opt/arcvpn/bin/arcvpn-core help)" || return 1
 if [[ "$capabilities" != *'local management protocol: lifecycle-v1'* ]]; then
   printf 'Core не поддерживает lifecycle-v1. Нужен согласованный пакет core/wrapper/units.\n' >&2
   return 1
 fi
 /opt/arcvpn/bin/arcvpn-core lifecycle "$@"
}
show_settings(){
 local panel_pid
 panel_pid="$(systemctl show arcvpn-panel.service --property=MainPID --value 2>/dev/null || true)"
 [[ "$panel_pid" =~ ^[0-9]+$ ]] || panel_pid=0
 run_core settings --panel-pid "$panel_pid" "$@"
}
management_help(){
 printf '%s\n' 'arcvpn — локальное управление' \
 '  arcvpn                         интерактивное меню' \
 '  arcvpn start|stop|restart       вся локальная установка' \
 '  arcvpn status [--json]         desired/actual каждого компонента' \
 '  arcvpn restart panel|xray|hysteria|wireguard' \
 '  arcvpn logs [panel|xray|hysteria|wireguard]   последние 80 записей' \
 '  arcvpn settings [--json]       настройки без секретов' \
 '  arcvpn password reset         новый пароль, 2FA сохраняется' \
 '  arcvpn password reset --password-stdin   пароль из stdin, 16–256 байт' \
 '  arcvpn password reset --reset-2fa        сбросить также 2FA' \
 '  arcvpn sessions revoke        завершить административные сессии' \
 '  arcvpn doctor                 диагностика' \
 'stop сохраняет данные; SSH, удалённые ноды и таймеры renewal/backup не меняются.'
}
case "${1:-menu}" in
 menu) [[ $# -le 1 ]] || { management_help;exit 2; };;
 help|--help|-h) management_help;exit 0;;
 start|stop) [[ $# -eq 1 ]] || { management_help;exit 2; };run_lifecycle "$@";exit $?;;
 status) [[ $# -eq 1 || ( $# -eq 2 && "$2" == --json ) ]] || { management_help;exit 2; };run_lifecycle "$@";exit $?;;
 settings) shift;show_settings "$@";exit $?;;
 password) shift;[[ "${1:-}" == reset ]] || { management_help;exit 2; };shift;run_core admin-reset "$@";exit $?;;
 admin-reset) shift;run_core admin-reset "$@";exit $?;;
 sessions) [[ $# -eq 2 && "$2" == revoke ]] || { management_help;exit 2; };run_core admin-revoke-sessions;exit $?;;
 restart|logs)
   [[ $# -ge 1 && $# -le 2 ]] || { management_help;exit 2; }
   case "${2:-}" in ''|panel|xray|hysteria|wireguard) ;; *) management_help;exit 2;;esac
   run_lifecycle "$@"
   exit $?;;
 doctor) [[ $# -eq 1 ]] || { management_help;exit 2; };run_core doctor;exit $?;;
 *) management_help;exit 2;;
esac

while true;do
 printf '\nArcVPN — управление\n1. Статус всех компонентов\n2. Перезапустить панель\n3. Перезапустить Xray\n4. Журнал панели\n5. Диагностика\n6. Зашифрованная резервная копия VPN\n7. Сбросить пароль администратора (сохранить 2FA)\n8. Настроить ежедневные зашифрованные копии\n9. Посмотреть настройки\n10. Завершить административные сессии\n11. Сбросить пароль и утраченную 2FA\n12. Запустить локальную установку\n13. Остановить локальную установку (данные сохраняются)\n14. Перезапустить локальную установку\n0. Выход\n> '
 read -r choice || exit 0
 case "$choice" in
 1) run_lifecycle status || true;;
 2) run_lifecycle restart panel || true;;
 3) run_lifecycle restart xray || true;;
 4) run_lifecycle logs panel || true;;
 5) runuser -u arcvpn -- env ARCVPN_DATA_DIR=/var/lib/arcvpn /opt/arcvpn/bin/arcvpn-core doctor;;
 6)
    printf 'Пароль шифрования (не менее 12 символов): '
    IFS= read -r -s secret || { printf '\n';continue; }
    printf '\nПодтвердить пароль: '
    IFS= read -r -s verify || { printf '\n';unset secret;continue; }
    printf '\n'
    if [[ ${#secret} -lt 12 || "$secret" != "$verify" ]];then printf 'Пароли не совпадают или слишком короткие\n';unset secret verify;continue;fi
    install -d -m 0700 -o arcvpn -g arcvpn /var/backups/arcvpn
    printf '%s\n' "$secret" | runuser -u arcvpn -- env ARCVPN_DATA_DIR=/var/lib/arcvpn /opt/arcvpn/bin/arcvpn-core backup-create --password-stdin --directory /var/backups/arcvpn --keep 7
    unset secret verify
    ;;
 7) printf 'Сбросить пароль администратора и завершить ВСЕ сессии? 2FA сохранится. [yes/no] ';read -r ok; if [[ "$ok" == yes ]];then runuser -u arcvpn -- env ARCVPN_DATA_DIR=/var/lib/arcvpn /opt/arcvpn/bin/arcvpn-core admin-reset;fi;;
 8)
    printf 'Новый пароль для ежедневных резервных копий (не менее 12 символов): '
    IFS= read -r -s secret || { printf '\n';continue; }
    printf '\nПодтверждение: '
    IFS= read -r -s verify || { printf '\n';unset secret;continue; }
    printf '\n'
    if [[ ${#secret} -lt 12 || "$secret" != "$verify" ]];then printf 'Пароли не совпадают или слишком короткие\n';unset secret verify;continue;fi
    tmp="$(mktemp /var/lib/arcvpn/.backup-password.XXXXXXXX)"
    chmod 0600 "$tmp"
    printf '%s\n' "$secret" > "$tmp"
    chown arcvpn:arcvpn "$tmp"
    mv -f "$tmp" /var/lib/arcvpn/backup-password
    unset secret verify
    systemctl enable --now arcvpn-backup.timer
    systemctl start arcvpn-backup.service
    echo 'Ежедневные зашифрованные копии включены. Регулярно копируй файлы /var/backups/arcvpn на другое устройство.'
    ;;
 9) show_settings || printf 'Не удалось прочитать настройки\n';;
 10) printf 'Завершить все административные сессии? VPN продолжит работу. [yes/no] ';read -r ok; if [[ "$ok" == yes ]];then run_core admin-revoke-sessions;fi;;
 11) printf 'Сбросить пароль и удалить 2FA? Для подтверждения введите RESET: ';read -r ok; if [[ "$ok" == RESET ]];then run_core admin-reset --reset-2fa;fi;;
 12) run_lifecycle start || true;;
 13) run_lifecycle stop || true;;
 14) run_lifecycle restart || true;;
 0) exit 0;;
 *) printf 'Неизвестная команда\n';;
 esac
done
