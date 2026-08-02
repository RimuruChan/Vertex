#!/bin/sh
set -eu

cg_root=/sys/fs/cgroup/isolate
controllers_file=/sys/fs/cgroup/cgroup.controllers

if [ ! -f "$controllers_file" ]; then
  echo "judge requires a writable cgroup v2 mount" >&2
  exit 1
fi

mkdir -p "$cg_root"
controllers=
for controller in memory pids cpu io cpuset; do
  if grep -qw "$controller" "$controllers_file"; then
    controllers="$controllers +$controller"
  fi
done
if [ -n "$controllers" ]; then
  echo "$controllers" > "$cg_root/cgroup.subtree_control"
fi

exec "$@"
