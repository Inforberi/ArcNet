#!/usr/bin/env bash
# Legacy installer is intentionally disabled. It used an enrollment secret in
# argv and downloaded its checksum from the same untrusted transport as the
# executable. Neither behavior meets the ArcVPN stage-4 security model.
set -Eeuo pipefail
printf '%s\n' 'ArcVPN: legacy /node-install.sh is disabled.' 'Use the SSH wizard with an independently signed agent/Xray bundle, or' 'follow docs/ops/stage4-nodes.md for restricted manual registration.' >&2
exit 64
