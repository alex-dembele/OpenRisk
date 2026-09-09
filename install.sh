#!/usr/bin/env bash
# OpenRisk — installation en une seule commande.
#
#   git clone https://github.com/opendefender/OpenRisk.git
#   cd OpenRisk
#   ./install.sh
#
# Ce fichier est volontairement à la racine : c'est la première chose que voit
# quelqu'un qui vient de cloner le dépôt, et lui demander de deviner
# `scripts/install.sh` est déjà une étape manuelle de trop (#328).
#
# Tout le travail réel est dans scripts/install.sh. Ce wrapper ne fait que le
# retrouver, où que vous soyez quand vous l'appelez.
set -euo pipefail

# Résout le répertoire de CE script en suivant les liens symboliques, pour que
# `~/bin/openrisk -> /srv/OpenRisk/install.sh` fonctionne aussi.
target="${BASH_SOURCE[0]}"
while [ -L "$target" ]; do
  dir="$(cd -P "$(dirname "$target")" && pwd)"
  target="$(readlink "$target")"
  [[ $target != /* ]] && target="$dir/$target"
done
ROOT="$(cd -P "$(dirname "$target")" && pwd)"

real="$ROOT/scripts/install.sh"
if [ ! -f "$real" ]; then
  printf '\033[1;31m[openrisk]\033[0m %s\n' \
    "scripts/install.sh est introuvable — ce dépôt semble incomplet." >&2
  exit 1
fi

# `exec` : l'installeur remplace ce processus, donc son code de sortie et ses
# signaux (Ctrl-C) sont les nôtres, sans couche intermédiaire.
exec bash "$real" "$@"
