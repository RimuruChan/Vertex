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
for path in "$sandbox_parent" "$scratch_parent"; do
  case "$path" in
    /*) ;;
    *) fail "instance root must be absolute: $path" ;;
  esac
done

# Resolve this container's cgroup from the namespace-relative membership, never
# create a sibling under the host root. Privileged runtimes expose cgroup v2
# read-write; a private cgroup namespace reports / and works the same way.
cg_relative=$(sed -n 's/^0:://p' /proc/self/cgroup)
case "$cg_relative" in
  /*) ;;
  *) fail 'cgroup v2 is required (no unified membership in /proc/self/cgroup)' ;;
esac
case "$cg_relative" in
  *'/../'*|*/..|*' (deleted)') fail 'container cgroup is outside the visible hierarchy' ;;
esac
cg_container=/sys/fs/cgroup${cg_relative%/}
# runc exec joins the init process's current cgroup. Re-entry must use the
# original container root rather than recursively creating manager subtrees.
case "$cg_container" in
  */vertex-manager) cg_container=${cg_container%/vertex-manager} ;;
esac
cg_parent=$cg_container/vertex-jobs
controllers_file=$cg_container/cgroup.controllers
subtree_file=$cg_container/cgroup.subtree_control
if [ ! -f "$controllers_file" ] || [ ! -w "$subtree_file" ]; then
  fail 'a writable cgroup v2 mount is required at /sys/fs/cgroup; run the worker container privileged'
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

# cgroup v2 forbids resident processes in a domain with enabled controllers.
# Move only this container's processes into its manager leaf before enabling
# child groups. All descendants (manager and jobs) retain the container limits.
manager_cgroup=$cg_container/vertex-manager
mkdir -p "$manager_cgroup" || fail "cannot create $manager_cgroup"
while IFS= read -r pid; do
  [ "$pid" -gt 0 ] || fail 'container cgroup contains processes outside its PID namespace'
  if ! printf '%s\n' "$pid" > "$manager_cgroup/cgroup.procs"; then
    [ ! -e "/proc/$pid" ] || fail "cannot move process $pid into manager cgroup"
  fi
done < "$cg_container/cgroup.procs"
# Do not write cpuset limits on the container root: nsdelegate protects those
# files in a private cgroup namespace. Children inherit its effective CPU set.
printf '%s\n' "$controllers" > "$subtree_file" || fail "cannot enable controllers under $cg_container"
mkdir -p "$cg_parent" || fail "cannot create $cg_parent"
if [ -n "${SANDBOX_CPUSET:-}" ]; then
  initialize_cpuset "$cg_parent"
fi
printf '%s\n' "$controllers" > "$cg_parent/cgroup.subtree_control" || fail "cannot enable controllers under $cg_parent"

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
export VERTEX_CONTAINER_CGROUP=$cg_container

# Keep the Go worker as PID 1 so SIGTERM/SIGINT reach its existing graceful
# shutdown path. Instance roots are intentionally retained and safely reused
# after a container restart; each native box reaps its own stale cgroup.
exec "$@"
