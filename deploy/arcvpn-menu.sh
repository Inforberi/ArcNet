#!/usr/bin/env bash
set -Eeuo pipefail
if [[ $EUID -ne 0 ]];then exec sudo -- "$0" "$@";fi

run_core(){ runuser -u arcvpn -- env ARCVPN_DATA_DIR=/var/lib/arcvpn /opt/arcvpn/bin/arcvpn-core "$@"; }
run_setup(){
 local capabilities
 capabilities="$(/opt/arcvpn/bin/arcvpn-core help)" || return 1
 [[ "$capabilities" == *'admin-setup'* ]] || { printf 'Нужен согласованный пакет новой панели и меню.\n' >&2;return 1; }
 run_core admin-setup
}
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
 '  arcvpn setup                    одноразовый код создания администратора' \
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
 setup) [[ $# -eq 1 ]] || { management_help;exit 2; };run_setup;exit $?;;
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

menu_page=main
arc_blue='' arc_green='' arc_red='' arc_clear=''
if [[ -t 1 && ${TERM:-dumb} != dumb && -z ${NO_COLOR:-} ]];then
 arc_blue=$'\033[1;34m' arc_green=$'\033[32m' arc_red=$'\033[31m' arc_clear=$'\033[0m'
fi
while true;do
 if [[ "$menu_page" == main ]];then
  panel_state="$(systemctl is-active arcvpn-panel.service 2>/dev/null || true)"
  panel_color="$arc_red"; [[ "$panel_state" != active ]] || panel_color="$arc_green"
  printf '\n%sArcVPN%s\nПанель: %s%s%s\n' "$arc_blue" "$arc_clear" "$panel_color" "${panel_state:-неизвестно}" "$arc_clear"
  printf '1. Службы\n2. Доступ и безопасность\n3. Резервные копии\n4. Диагностика и журналы\n5. Настройки панели\n0. Выход\n> '
 else
  case "$menu_page" in
   services) printf '\n%sСлужбы%s\n1. Статус\n2. Запустить VPN\n3. Остановить VPN (данные сохраняются)\n4. Перезапустить VPN\n5. Перезапустить панель\n6. Перезапустить Xray\n' "$arc_blue" "$arc_clear";;
   access) printf '\n%sДоступ и безопасность%s\n1. Адрес и настройки панели\n2. Код первого запуска\n3. Новый пароль (сохранить 2FA)\n4. Завершить сессии\n5. Восстановить доступ при утрате 2FA\n' "$arc_blue" "$arc_clear";;
   backup) printf '\n%sРезервные копии%s\n1. Создать зашифрованную копию\n2. Настроить ежедневные копии\n' "$arc_blue" "$arc_clear";;
   diagnostics) printf '\n%sДиагностика%s\n1. Проверить установку\n2. Последние записи панели\n3. Статус всех служб\n' "$arc_blue" "$arc_clear";;
  esac
  printf '0. Назад\n> '
 fi
 read -r choice || exit 0
 if [[ "$menu_page" == main ]];then
  case "$choice" in
   1) menu_page=services;continue;; 2) menu_page=access;continue;;
   3) menu_page=backup;continue;; 4) menu_page=diagnostics;continue;;
   5) choice=9;; 0) exit 0;; *) printf 'Выберите пункт из меню.\n';continue;;
  esac
 else
  if [[ "$choice" == 0 ]];then menu_page=main;continue;fi
  case "$menu_page:$choice" in
   services:1|diagnostics:3) choice=1;; services:2) choice=12;; services:3) choice=13;;
   services:4) choice=14;; services:5) choice=2;; services:6) choice=3;;
   access:1) choice=9;; access:2) choice=15;; access:3) choice=7;; access:4) choice=10;; access:5) choice=11;;
   backup:1) choice=6;; backup:2) choice=8;; diagnostics:1) choice=5;; diagnostics:2) choice=4;;
   *) printf 'Выберите пункт из меню.\n';continue;;
  esac
 fi
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
 15) run_setup || printf "Код доступен только до создания администратора. Для смены пароля используйте раздел доступа.\n";;
 0) exit 0;;
 *) printf 'Неизвестная команда\n';;
 esac
done
