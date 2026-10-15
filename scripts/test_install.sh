#!/usr/bin/env bash
# Exercises input and existing-Docker failure paths without installing any software.
set -Eeuo pipefail
cd -- "$(dirname -- "$0")/.."
# Definitions only: the final main invocation is deliberately not sourced.
source <(sed '$d' install.sh)
read_install_options --host vpn.example.com --port 02053
validate_install_address
[[ "$PANEL_HOST" == vpn.example.com && "$PANEL_PORT" == 2053 ]]
reject(){ if ( "$@" ) >/dev/null 2>&1; then echo "Unexpected success: $*" >&2; exit 1; fi; }
reject read_install_options --host
reject read_install_options --unknown
for PANEL_HOST in 'https://vpn.example.com' '-x' 'bad..host' 'host/path' 'host:443' 'host;touch x'; do
 PANEL_PORT=2053; reject validate_install_address
done
PANEL_HOST=192.0.2.1
for PANEL_PORT in 0 65536 -1 '1+2' 000000 12x; do reject validate_install_address; done
docker(){ [[ "$*" == 'compose version' ]]; }
reject install_docker # missing Buildx must not replace an existing Docker installation
docker(){ [[ "$*" == 'compose version' || "$*" == 'buildx version' ]]; }
reject install_docker # unreachable daemon must stop before cloning/building
docker(){ return 0; }
install_docker
printf 'Bootstrap input and existing-Docker checks passed.\n'
