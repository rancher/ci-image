#!/bin/sh
# ci-env-init — auto-configure CI tool families from environment variables.
#
# Scans for {FAMILY}_VERSION environment variables and automatically runs
# ci-select for each one. After configuration, execs the user's command.
#
# Usage (set as ENTRYPOINT):
#   docker run -e HELM_VERSION=helmv4 image helm version
#
# The ENTRYPOINT will configure helm to point to helmv4, then run "helm version".

set -e

FAMILIES_DIR=/usr/local/share/ci-tools/families

# Auto-configure families based on {FAMILY}_VERSION env vars
if [ -d "${FAMILIES_DIR}" ]; then
    for _family_dir in "${FAMILIES_DIR}"/*/; do
        [ -d "${_family_dir}" ] || continue
        _family=$(basename "${_family_dir}")

        # Convert family name to uppercase with underscores for env var
        _env_var=$(printf '%s' "${_family}" | tr '[:lower:]-' '[:upper:]_')_VERSION

        # Use eval to safely get the env var value
        eval "_tool=\${${_env_var}:-}"

        if [ -n "${_tool}" ]; then
            printf '[ci-env-init] Configuring %s -> %s\n' "${_family}" "${_tool}" >&2
            if ! ci-select "${_family}" "${_tool}"; then
                printf '[ci-env-init] ERROR: Failed to configure %s=%s\n' "${_env_var}" "${_tool}" >&2
                exit 1
            fi
        fi
    done
fi

# Exec the user's command, preserving PID 1
exec "$@"