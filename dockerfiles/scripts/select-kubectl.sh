#!/bin/sh
# select-kubectl — activate a kubectl version for this CI job.
# Equivalent to: ci-select kubectl [TOOL]
#
# Usage:
#   select-kubectl              show available tools and current selection
#   select-kubectl TOOL         activate TOOL as the default 'kubectl' command
exec ci-select kubectl "$@"