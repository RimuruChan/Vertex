#!/bin/sh
set -eu

fail() {
  echo "worker entrypoint: $*" >&2
  exit 1
}

instance_id=${SANDBOX_INSTANCE_ID:-${HOSTNAME:-}}
if [ -z "$instance_id" ] && [ -r /etc/hostname ]; then
  instance_id=$(sed -n '1p' /etc/hostname)
fi
case "$instance_id" in
  ''|[!A-Za-z0-9]*|*[!A-Za-z0-9_.-]*)
    fail 'SANDBOX_INSTANCE_ID must start with an alphanumeric character and contain only A-Z, a-z, 0-9, _, . or -'
    ;;
esac
if [ "${#instance_id}" -gt 64 ]; then
  fail 'SANDBOX_INSTANCE_ID must contain at most 64 characters'
fi

sandbox_parent=${SANDBOX_BASE:-/var/local/lib/vertex-sandbox}
scratch_parent=${SCRATCH_ROOT:-/scratch}
cg_parent=${VERTEX_CGROUP_ROOT:-/vertex-cgroup}
for path in "$sandbox_parent" "$scratch_parent" "$cg_parent"; do
  case "$path" in
    /*) ;;
    *) fail "instance root must be absolute: $path" ;;
  esac
done

controllers_file=$cg_parent/cgroup.controllers
subtree_file=$cg_parent/cgroup.subtree_control
if [ ! -f "$controllers_file" ] || [ ! -f "$subtree_file" ]; then
  fail "writable delegated cgroup v2 root is required at $cg_parent"
fi

controllers=
for controller in cpu memory pids; do
  if ! grep -qw "$controller" "$controllers_file"; then
    fail "required cgroup controller is not delegated: $controller"
  fi
  controllers="$controllers +$controller"
done
if [ -n "${SANDBOX_CPUSET:-}" ]; then
  if ! grep -qw cpuset "$controllers_file"; then
    fail 'SANDBOX_CPUSET requires the cpuset cgroup controller'
  fi
  controllers="$controllers +cpuset"
fi

initialize_cpuset() {
  root=$1
  for dimension in cpus mems; do
    configured=$root/cpuset.$dimension
    effective=$root/cpuset.$dimension.effective
    [ -f "$configured" ] && [ -f "$effective" ] || fail "cpuset files are unavailable under $root"
    value=$(tr -d '[:space:]' < "$configured")
    if [ -z "$value" ]; then
      value=$(tr -d '[:space:]' < "$effective")
      [ -n "$value" ] || fail "cpuset.$dimension.effective is empty under $root"
      printf '%s\n' "$value" > "$configured" || fail "cannot initialize $configured"
    fi
  done
}

if [ -n "${SANDBOX_CPUSET:-}" ]; then
  initialize_cpuset "$cg_parent"
fi
printf '%s\n' "$controllers" > "$subtree_file" || fail "cannot enable controllers under $cg_parent"

instance_cgroup=$cg_parent/instance-$instance_id
mkdir -p "$instance_cgroup" || fail "cannot create instance cgroup $instance_cgroup"
if [ -n "${SANDBOX_CPUSET:-}" ]; then
  initialize_cpuset "$instance_cgroup"
fi
printf '%s\n' "$controllers" > "$instance_cgroup/cgroup.subtree_control" || \
  fail "cannot enable controllers under $instance_cgroup"

for controller in cpu memory pids; do
  grep -qw "$controller" "$instance_cgroup/cgroup.controllers" || \
    fail "instance cgroup does not expose controller: $controller"
  grep -qw "$controller" "$instance_cgroup/cgroup.subtree_control" || \
    fail "instance cgroup did not enable controller: $controller"
done
if [ -n "${SANDBOX_CPUSET:-}" ]; then
  grep -qw cpuset "$instance_cgroup/cgroup.controllers" || fail 'instance cgroup does not expose cpuset'
  grep -qw cpuset "$instance_cgroup/cgroup.subtree_control" || fail 'instance cgroup did not enable cpuset'
fi

instance_sandbox=$sandbox_parent/instances/$instance_id
instance_scratch=$scratch_parent/instances/$instance_id
instance_lock_dir=$sandbox_parent/instances/.locks
instance_lock=$instance_lock_dir/$instance_id.lock
mkdir -p "$instance_sandbox" "$instance_scratch" "$instance_lock_dir" || \
  fail 'cannot create instance workspace directories'

export SANDBOX_INSTANCE_ID=$instance_id
export SANDBOX_INSTANCE_LOCK=$instance_lock
export SANDBOX_BASE=$instance_sandbox
export SCRATCH_ROOT=$instance_scratch
export VERTEX_CGROUP_ROOT=$instance_cgroup

# Keep the Go worker as PID 1 so SIGTERM/SIGINT reach its existing graceful
# shutdown path. Instance roots are intentionally retained and safely reused
# after a container restart; each native box reaps its own stale cgroup.
exec "$@"
