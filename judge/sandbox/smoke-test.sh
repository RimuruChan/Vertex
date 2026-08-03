#!/bin/sh
set -eu

base=${SANDBOX_BASE:-/var/local/lib/vertex-sandbox}
box_id=31
cg_root=${VERTEX_CGROUP_ROOT:-/vertex-cgroup}

cleanup() {
  vertex-sandbox cleanup --box-id "$box_id" --base "$base" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

# Docker's general cgroupfs must stay read-only; only the project subtree is rw.
main_cgroup_options=$(awk '$2 == "/sys/fs/cgroup" { print $4; exit }' /proc/mounts)
project_cgroup_options=$(awk '$2 == "/vertex-cgroup" { print $4; exit }' /proc/mounts)
case ",$main_cgroup_options," in
  *,ro,*) ;;
  *) echo "host cgroupfs is not read-only" >&2; exit 1 ;;
esac
case ",$project_cgroup_options," in
  *,rw,*) ;;
  *) echo "project cgroup subtree is not writable" >&2; exit 1 ;;
esac

vertex-sandbox probe
vertex-sandbox init --box-id "$box_id" --base "$base"

run() {
  vertex-sandbox run \
    --box-id "$box_id" \
    --base "$base" \
    --time-ms 2000 \
    --time-hard-ms 3000 \
    --wall-ms 4000 \
    --wall-hard-ms 5000 \
    --memory-kb 262144 \
    --processes 8 \
    --output-bytes 1048576 \
    --workspace-bytes 67108864 \
    --workspace-inodes 4096 \
    -- "$@"
}

# Files outside the workspace, network sockets, and the worker environment must
# all be unavailable to the submission process.
run /usr/bin/python3 -c 'import os, socket
filesystem_denied = False
outside_write_denied = False
network_denied = False
try:
    open("/testdata")
except PermissionError:
    filesystem_denied = True
try:
    open("/scratch/vertex-sandbox-escape", "w")
except PermissionError:
    outside_write_denied = True
try:
    socket.socket()
except PermissionError:
    network_denied = True
print("isolated" if filesystem_denied and outside_write_denied and network_denied and os.getenv("DATABASE_URL") is None and os.geteuid() == 60031 else "unsafe")'
test "$(cat "$base/$box_id/control/stdout")" = "isolated"
test ! -e /scratch/vertex-sandbox-escape

# Exercise a real C++ toolchain under Landlock/seccomp, then execute its output.
run /usr/bin/python3 -c 'open("main.cpp", "w").write("#include <iostream>\nint main(){std::cout << 42;}\n")'
run /usr/bin/g++ -O2 -std=c++17 -o prog main.cpp
run ./prog
test "$(cat "$base/$box_id/control/stdout")" = "42"

if grep -qw cpuset "$cg_root/cgroup.controllers" &&
   grep -qw cpuset "$cg_root/cgroup.subtree_control"; then
  effective_cpus=$(cat "$cg_root/cpuset.cpus.effective")
  first_cpu=${effective_cpus%%[-,]*}
  vertex-sandbox run \
    --box-id "$box_id" --base "$base" \
    --time-ms 2000 --time-hard-ms 3000 \
    --wall-ms 4000 --wall-hard-ms 5000 --memory-kb 262144 \
    --processes 8 --output-bytes 1048576 \
    --workspace-bytes 67108864 --workspace-inodes 4096 \
    --cpu-set "$first_cpu" \
    -- /usr/bin/python3 -c 'import os; print(next(iter(os.sched_getaffinity(0))))'
  test "$(cat "$base/$box_id/control/stdout")" = "$first_cpu"
fi

# A submission cannot daemonize past the supervised main process.
run /usr/bin/python3 -c 'import os, time
pid = os.fork()
if pid:
    with open("child.pid", "w") as output:
        output.write(str(pid))
    os._exit(0)
time.sleep(60)'
child_pid=$(cat "$base/$box_id/box/child.pid")
if kill -0 "$child_pid" 2>/dev/null; then
  echo "sandbox descendant survived cgroup cleanup" >&2
  exit 1
fi

vertex-sandbox run \
  --box-id "$box_id" --base "$base" \
  --time-ms 100 --time-hard-ms 1000 \
  --wall-ms 1000 --wall-hard-ms 2000 --memory-kb 262144 \
  --processes 8 --output-bytes 1048576 \
  --workspace-bytes 67108864 --workspace-inodes 4096 \
  -- /usr/bin/python3 -c 'import time
while time.process_time() < 0.25: pass'
grep -q '^status:TO$' "$base/$box_id/control/meta"
grep -q '^killed:0$' "$base/$box_id/control/meta"

vertex-sandbox run \
  --box-id "$box_id" --base "$base" \
  --time-ms 100 --time-hard-ms 300 \
  --wall-ms 1000 --wall-hard-ms 2000 --memory-kb 262144 \
  --processes 8 --output-bytes 1048576 \
  --workspace-bytes 67108864 --workspace-inodes 4096 \
  -- /usr/bin/python3 -c 'while True: pass'
grep -q '^status:TO$' "$base/$box_id/control/meta"
grep -q '^killed:1$' "$base/$box_id/control/meta"

vertex-sandbox run \
  --box-id "$box_id" --base "$base" \
  --time-ms 2000 --time-hard-ms 3000 \
  --wall-ms 100 --wall-hard-ms 1000 --memory-kb 262144 \
  --processes 8 --output-bytes 1048576 \
  --workspace-bytes 67108864 --workspace-inodes 4096 \
  -- /usr/bin/python3 -c 'import time; time.sleep(0.25)'
grep -q '^status:TO$' "$base/$box_id/control/meta"
grep -q '^killed:0$' "$base/$box_id/control/meta"

vertex-sandbox run \
  --box-id "$box_id" --base "$base" \
  --time-ms 2000 --time-hard-ms 3000 \
  --wall-ms 100 --wall-hard-ms 300 --memory-kb 262144 \
  --processes 8 --output-bytes 1048576 \
  --workspace-bytes 67108864 --workspace-inodes 4096 \
  -- /usr/bin/python3 -c 'import time; time.sleep(60)'
grep -q '^status:TO$' "$base/$box_id/control/meta"
grep -q '^killed:1$' "$base/$box_id/control/meta"

vertex-sandbox run \
  --box-id "$box_id" --base "$base" \
  --time-ms 2000 --time-hard-ms 3000 \
  --wall-ms 4000 --wall-hard-ms 5000 --memory-kb 262144 \
  --processes 8 --output-bytes 65536 \
  --workspace-bytes 67108864 --workspace-inodes 4096 \
  -- /usr/bin/python3 -c 'import os; os.write(1, b"x" * 131072)'
grep -q '^exitsig:25$' "$base/$box_id/control/meta"
grep -q '^output-limit:1$' "$base/$box_id/control/meta"

vertex-sandbox run \
  --box-id "$box_id" --base "$base" \
  --time-ms 2000 --time-hard-ms 3000 \
  --wall-ms 4000 --wall-hard-ms 5000 --memory-kb 32768 \
  --processes 8 --output-bytes 1048576 \
  --workspace-bytes 67108864 --workspace-inodes 4096 \
  -- /usr/bin/python3 -c 'bytearray(256 * 1024 * 1024)'
grep -q '^cg-oom-killed:1$' "$base/$box_id/control/meta"

vertex-sandbox cleanup --box-id "$box_id" --base "$base"
vertex-sandbox init --box-id "$box_id" --base "$base"
vertex-sandbox run \
  --box-id "$box_id" --base "$base" \
  --time-ms 2000 --time-hard-ms 3000 \
  --wall-ms 4000 --wall-hard-ms 5000 --memory-kb 262144 \
  --processes 8 --output-bytes 1048576 \
  --workspace-bytes 1048576 --workspace-inodes 4096 \
  -- /usr/bin/python3 -c 'data = b"x" * 16384
for i in range(256):
    open(f"bytes-{i}", "wb").write(data)'
grep -q '^workspace-limit:1$' "$base/$box_id/control/meta"
grep -q '^killed:1$' "$base/$box_id/control/meta"

vertex-sandbox cleanup --box-id "$box_id" --base "$base"
vertex-sandbox init --box-id "$box_id" --base "$base"
vertex-sandbox run \
  --box-id "$box_id" --base "$base" \
  --time-ms 2000 --time-hard-ms 3000 \
  --wall-ms 4000 --wall-hard-ms 5000 --memory-kb 262144 \
  --processes 8 --output-bytes 1048576 \
  --workspace-bytes 67108864 --workspace-inodes 64 \
  -- /usr/bin/python3 -c 'for i in range(256): open(f"inode-{i}", "wb").close()'
grep -q '^workspace-limit:1$' "$base/$box_id/control/meta"
awk -F: '$1 == "workspace-inodes" && $2 >= 64 { found=1 } END { exit !found }' \
  "$base/$box_id/control/meta"
test ! -e "$cg_root/box-$box_id"

echo "vertex-sandbox smoke test passed"
