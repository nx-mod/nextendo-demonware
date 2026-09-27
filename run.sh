#!/usr/bin/env sh
# nextendo-demonware — home-lab launch (Diablo III + CTR Demonware backend).
# cert.pem/key.pem and nextendo_secret.key ship on this testing branch, so it
# runs straight from the repo. Auth is HTTPS on AUTH_PORT (8460 by default,
# behind the sni-router that fronts :443). State dirs are created on demand.
set -e
export CERT_FILE="${CERT_FILE:-cert.pem}"
export KEY_FILE="${KEY_FILE:-key.pem}"
export D3_STATE="${D3_STATE:-state}"
export AUTH_PORT="${AUTH_PORT:-8460}"
export NAT_PORT="${NAT_PORT:-3074}"
export DASH_PORT="${DASH_PORT:-8093}"
export NEXTENDO_SECRET_FILE="${NEXTENDO_SECRET_FILE:-nextendo_secret.key}"
mkdir -p "$D3_STATE"
echo "[demonware] auth :$AUTH_PORT  nat :$NAT_PORT  dash :$DASH_PORT  state $D3_STATE"
exec go run .
