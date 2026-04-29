#!/bin/sh
# select-helm — activate a helm version for this CI job.
# Equivalent to: ci-select helm [TOOL]
#
# Usage:
#   select-helm              show available tools and current selection
#   select-helm TOOL         activate TOOL as the default 'helm' command
exec ci-select helm "$@"
