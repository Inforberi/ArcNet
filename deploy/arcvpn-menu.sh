#!/usr/bin/env bash
set -Eeuo pipefail
if [[ $EUID -ne 0 ]];then exec sudo "$0" "$@";fi
while true;do
 printf '\nArcVPN — управление\n1. Статус\n2. Перезапустить панель\n3. Перезапустить Xray\n4. Журнал панели\n5. Диагностика\n6. Зашифрованная резервная копия VPN\n7. Сбросить пароль и 2FA администратора\n8. Настроить ежедневные зашифрованные копии\n0. Выход\n> '
 read -r choice || exit 0
 case "$choice" in
 1) systemctl --no-pager status arcvpn-panel.service arcvpn-xray.service || true;;
 2) systemctl restart arcvpn-panel.service;;
 3) systemctl restart arcvpn-xray.service;;
 4) journalctl --no-pager -n 80 -u arcvpn-panel.service;;
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
 7) printf 'Сбросить пароль и 2FA администратора и завершить ВСЕ сессии? [yes/no] ';read -r ok; if [[ "$ok" == yes ]];then runuser -u arcvpn -- env ARCVPN_DATA_DIR=/var/lib/arcvpn /opt/arcvpn/bin/arcvpn-core admin-reset;fi;;
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
 0) exit 0;;
 *) printf 'Неизвестная команда\n';;
 esac
done
