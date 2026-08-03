#!/bin/sh
set -eu

cg_root=${VERTEX_CGROUP_ROOT:-/vertex-cgroup}
controllers_file=$cg_root/cgroup.controllers

if [ ! -f "$controllers_file" ]; then
  echo "judge requires a writable cgroup v2 mount" >&2
  exit 1
fi

mkdir -p "$cg_root"
controllers=
for controller in memory pids cpu; do
  if grep -qw "$controller" "$controllers_file"; then
    controllers="$controllers +$controller"
  fi
done
if [ -n "${SANDBOX_CPUSET:-}" ]; then
  if ! grep -qw cpuset "$controllers_file"; then
    echo "SANDBOX_CPUSET requires the cpuset cgroup controller" >&2
    exit 1
  fi
  controllers="$controllers +cpuset"
fi
if [ -n "$controllers" ]; then
  echo "$controllers" > "$cg_root/cgroup.subtree_control"
fi

exec "$@"
