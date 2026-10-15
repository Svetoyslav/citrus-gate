#!/usr/bin/env bash
# Citrus Gate bootstrap for a fresh Ubuntu/Debian server. Source: Svetoyslav/citrus-gate.
set -Eeuo pipefail

die(){ printf 'Citrus Gate: %s\n' "$*" >&2; exit 1; }
usage(){
 cat <<'HELP'
Citrus Gate — установка с GitHub
  bash install.sh [--host DOMAIN_OR_IPV4] [--port 2053]
  bash install.sh --help

Запустите от root на чистом Ubuntu 22.04/24.04/26.04 или Debian 12/13.
Без параметров установщик спросит домен/IP и порт через терминал.
Код: https://github.com/Svetoyslav/citrus-gate, каталог: /opt/citrus-gate.
HELP
}

read_install_options(){
 PANEL_HOST=''; PANEL_PORT=2053
 while (($#)); do
  case "$1" in
   --help|-h) usage; exit 0 ;;
   --host) (($# >= 2)) || die 'После --host нужен домен или IPv4.'; PANEL_HOST=$2; shift 2 ;;
   --port) (($# >= 2)) || die 'После --port нужен номер порта.'; PANEL_PORT=$2; shift 2 ;;
   *) die "Неизвестный параметр: $1" ;;
  esac
 done
}

install_docker(){
 if command -v docker >/dev/null; then
  docker compose version >/dev/null 2>&1 || die 'Docker уже установлен, но нет Compose v2. Установите Compose v2 и повторите.'
  docker buildx version >/dev/null 2>&1 || die 'Docker уже установлен, но нет Buildx. Установите docker-buildx-plugin и повторите.'
 else
  # Do not remove another container runtime or its packages on an existing server.
  local pkg
  for pkg in docker.io docker-compose docker-compose-v2 podman-docker containerd runc; do
   if [[ $(dpkg-query -W -f='${Status}' "$pkg" 2>/dev/null || true) == 'install ok installed' ]]; then
    die "Найден пакет $pkg. Подготовьте Docker Engine и Compose по документации Docker; существующие пакеты автоматически не удаляются."
   fi
  done
  install -m 0755 -d /etc/apt/keyrings
  curl --proto '=https' --tlsv1.2 -fLsS --retry 3 "https://download.docker.com/linux/$OS_ID/gpg" -o /etc/apt/keyrings/citrus-docker.asc
  chmod 0644 /etc/apt/keyrings/citrus-docker.asc
  cat >/etc/apt/sources.list.d/citrus-docker.sources <<REPO
Types: deb
URIs: https://download.docker.com/linux/$OS_ID
Suites: $OS_CODENAME
Components: stable
Architectures: $(dpkg --print-architecture)
Signed-By: /etc/apt/keyrings/citrus-docker.asc
REPO
  apt-get update
  DEBIAN_FRONTEND=noninteractive apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
  systemctl enable --now docker
 fi
 docker info >/dev/null 2>&1 || die 'Docker не запущен или недоступен. Проверьте systemctl status docker.'
 docker compose version >/dev/null
 docker buildx version >/dev/null
}

validate_install_address(){
 [[ "$PANEL_HOST" =~ ^[a-zA-Z0-9][a-zA-Z0-9.-]*$ && "$PANEL_HOST" != *..* && ${#PANEL_HOST} -le 253 ]] || die 'Нужен домен или IPv4 без https://, пути и порта.'
 [[ "$PANEL_PORT" =~ ^[0-9]{1,5}$ ]] && ((10#$PANEL_PORT > 0 && 10#$PANEL_PORT <= 65535)) || die 'Порт должен быть от 1 до 65535.'
 PANEL_PORT=$((10#$PANEL_PORT))
}

main(){
 read_install_options "$@"
 [[ $(uname -s) == Linux ]] || die 'Установщик предназначен для Linux.'
 ((EUID == 0)) || die 'Войдите на сервер как root или запустите через sudo bash.'
 [[ ! -e /opt/citrus-gate && ! -L /opt/citrus-gate ]] || die '/opt/citrus-gate уже существует. Повторная установка не перезаписывает данные.'
 [[ -r /etc/os-release ]] || die 'Не удалось определить систему.'
 # /etc/os-release is the local operating system metadata, not a downloaded script.
 . /etc/os-release
 OS_ID=${ID:-}; OS_CODENAME=${VERSION_CODENAME:-}
 case "$OS_ID:${VERSION_ID:-}" in
  ubuntu:22.04|ubuntu:24.04|ubuntu:26.04|debian:12|debian:13) ;;
  *) die 'Поддерживаются Ubuntu 22.04/24.04/26.04 и Debian 12/13.' ;;
 esac
 [[ "$OS_CODENAME" =~ ^[a-z]+$ ]] || die 'Не удалось определить кодовое имя системы.'
 case "$(uname -m)" in x86_64|aarch64) ;; *) die 'Нужен сервер amd64 или arm64.' ;; esac
 command -v systemctl >/dev/null || die 'Для автоматической установки нужен systemd.'
 if [[ -z "$PANEL_HOST" ]]; then
  [[ -r /dev/tty ]] || die 'Укажите --host DOMAIN_OR_IPV4 для запуска без терминала.'
  read -r -p 'Домен панели или публичный IPv4: ' PANEL_HOST </dev/tty || die 'Не удалось прочитать адрес.'
  read -r -p 'Порт панели [2053]: ' choice </dev/tty || die 'Не удалось прочитать порт.'
  PANEL_PORT=${choice:-2053}
 fi
 validate_install_address
 printf '\nCitrus Gate: %s:%s, каталог /opt/citrus-gate\n' "$PANEL_HOST" "$PANEL_PORT"
 printf 'Установка зависимостей и сборка из исходников могут занять несколько минут.\n'
 apt-get update
 DEBIAN_FRONTEND=noninteractive apt-get install -y ca-certificates curl git python3 tar util-linux
 install_docker
 # clone refuses a concurrently created nonempty target; do not delete it on error.
 git clone --depth 1 --single-branch --branch main https://github.com/Svetoyslav/citrus-gate.git /opt/citrus-gate
 cd /opt/citrus-gate
 printf 'Версия исходников: '; git rev-parse HEAD
 chmod 0755 citrus
 bash citrus install "$PANEL_HOST" "$PANEL_PORT"
 printf '\nCitrus Gate установлен. Сохраните показанные выше ссылку, логин и пароль.\n'
 printf 'Управление: cd /opt/citrus-gate && bash citrus status\n'
}

# Keep this invocation last, so a partial download cannot start installation mid-file.
main "$@"
