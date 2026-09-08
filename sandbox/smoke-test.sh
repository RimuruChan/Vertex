#!/bin/sh
set -eu

# Use the native preparation API, just like any other sandbox client.
set -- prepare --base "${SANDBOX_BASE:-/vertex/sandbox}"
if [ -n "${SANDBOX_INSTANCE_ID:-}" ]; then
  set -- "$@" --instance-id "$SANDBOX_INSTANCE_ID"
fi
if [ -n "${SANDBOX_CPUSET:-}" ]; then
  set -- "$@" --cpu-set "$SANDBOX_CPUSET"
fi
runtime=$(vertex-sandbox "$@")
read_runtime() { printf '%s' "$runtime" | python3 -c 'import json, sys; print(json.load(sys.stdin)[sys.argv[1]])' "$1"; }
base=$(read_runtime base)
cg_root=$(read_runtime cgroupRoot)
container_cgroup=$(read_runtime containerCgroup)
instance_id=$(read_runtime instanceId)
box_id=31
duplex_box_id=32
duplex_left_to_right="$base/duplex-left-to-right"
duplex_right_to_left="$base/duplex-right-to-left"

cleanup() {
  vertex-sandbox cleanup --box-id "$box_id" --base "$base" >/dev/null 2>&1 || true
  vertex-sandbox cleanup --box-id "$duplex_box_id" --base "$base" >/dev/null 2>&1 || true
  rm -f "$duplex_left_to_right" "$duplex_right_to_left"
}
trap cleanup EXIT INT TERM

[ "$cg_root" = "$container_cgroup/vertex-jobs/instance-$instance_id" ]
[ -w "$cg_root/cgroup.subtree_control" ]
# Privileged applies to the supervisor only. Submitted code below must have
# no capabilities, no worker credentials and no filesystem/network escape.

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
    --env VERTEX_SMOKE_FIRST=one \
    --env VERTEX_SMOKE_SECOND=two \
    -- "$@"
}

# Files outside the workspace, network sockets, and the worker environment must
# all be unavailable to the submission process.
run /usr/bin/python3 -c 'import ctypes, os, socket
libc = ctypes.CDLL(None, use_errno=True)
class Header(ctypes.Structure):
    _fields_ = [("version", ctypes.c_uint32), ("pid", ctypes.c_int)]
class Caps(ctypes.Structure):
    _fields_ = [("effective", ctypes.c_uint32), ("permitted", ctypes.c_uint32), ("inheritable", ctypes.c_uint32)]
header = Header(0x20080522, 0)
caps = (Caps * 2)()
assert libc.capget(ctypes.byref(header), caps) == 0
assert all(c.effective == c.permitted == c.inheritable == 0 for c in caps)
assert libc.prctl(39, 0, 0, 0, 0) == 1  # PR_GET_NO_NEW_PRIVS
filesystem_denied = False
outside_write_denied = False
network_denied = False
try:
    open("/vertex/testdata")
except PermissionError:
    filesystem_denied = True
try:
    open("/vertex/scratch/vertex-sandbox-escape", "w")
except PermissionError:
    outside_write_denied = True
try:
    socket.socket()
except PermissionError:
    network_denied = True
safe = (
    filesystem_denied and outside_write_denied and network_denied
    and os.getenv("DATABASE_URL") is None
    and os.getenv("JUDGE_API_TOKEN") is None
    and os.getenv("VERTEX_SMOKE_FIRST") == "one"
    and os.getenv("VERTEX_SMOKE_SECOND") == "two"
    and os.geteuid() == 60031
)
print("isolated" if safe else "unsafe")'
test "$(cat "$base/$box_id/control/stdout")" = "isolated"
test ! -e /vertex/scratch/vertex-sandbox-escape
grep -q '^termination-reason:exited$' "$base/$box_id/control/meta"
grep -q '^time-result:none$' "$base/$box_id/control/meta"
awk -F: '$1 == "stdout-bytes" && $2 > 0 { found=1 } END { exit !found }' \
  "$base/$box_id/control/meta"

# Exercise a real C++ toolchain under Landlock/seccomp, then execute its output.
run /usr/bin/python3 -c 'open("main.cpp", "w").write("#include <iostream>\nint main(){std::cout << 42;}\n")'
run /usr/bin/g++ -O2 -std=c++17 -o prog main.cpp
run ./prog
test "$(cat "$base/$box_id/control/stdout")" = "42"

# Trusted orchestrators may attach inherited pipes for interactive traffic.
stream_input="$base/$box_id/box/stream-input"
stream_output="$base/$box_id/control/stream-output"
printf 'ping\n' >"$stream_input"
vertex-sandbox run \
  --box-id "$box_id" --base "$base" \
  --time-ms 2000 --time-hard-ms 3000 \
  --wall-ms 4000 --wall-hard-ms 5000 --memory-kb 262144 \
  --processes 8 --output-bytes 1048576 \
  --workspace-bytes 67108864 --workspace-inodes 4096 \
  --stdin-fd 3 --stdout-fd 4 \
  -- /usr/bin/python3 -c 'print(input().upper(), end="")' \
  3<"$stream_input" 4>"$stream_output"
test "$(cat "$stream_output")" = "PING"
test ! -e "$base/$box_id/control/stdout"
grep -q '^stdout-streamed:1$' "$base/$box_id/control/meta"

# Two independent sandboxes can exchange a request and response through only
# inherited pipes. The trusted shell wiring stands in for the Go stream broker;
# neither role shares a workspace, UID, or cgroup with its peer.
vertex-sandbox init --box-id "$duplex_box_id" --base "$base"
rm -f "$duplex_left_to_right" "$duplex_right_to_left"
mkfifo "$duplex_left_to_right" "$duplex_right_to_left"
exec 7<>"$duplex_left_to_right"
exec 8<>"$duplex_right_to_left"
vertex-sandbox run \
  --box-id "$box_id" --base "$base" \
  --time-ms 2000 --time-hard-ms 3000 \
  --wall-ms 4000 --wall-hard-ms 5000 --memory-kb 262144 \
  --processes 8 --output-bytes 1048576 \
  --workspace-bytes 67108864 --workspace-inodes 4096 \
  --stdin-fd 8 --stdout-fd 7 \
  -- /usr/bin/python3 -c 'print("ping", flush=True); assert input() == "PING"' \
  7>&7 8<&8 &
left_runner_pid=$!
vertex-sandbox run \
  --box-id "$duplex_box_id" --base "$base" \
  --time-ms 2000 --time-hard-ms 3000 \
  --wall-ms 4000 --wall-hard-ms 5000 --memory-kb 262144 \
  --processes 8 --output-bytes 1048576 \
  --workspace-bytes 67108864 --workspace-inodes 4096 \
  --stdin-fd 7 --stdout-fd 8 \
  -- /usr/bin/python3 -c 'print(input().upper(), flush=True)' \
  7<&7 8>&8 &
right_runner_pid=$!
wait "$left_runner_pid"
wait "$right_runner_pid"
exec 7>&-
exec 8>&-
grep -q '^termination-reason:exited$' "$base/$box_id/control/meta"
grep -q '^termination-reason:exited$' "$base/$duplex_box_id/control/meta"
test "$(stat -c %u "$base/$box_id/box")" != \
  "$(stat -c %u "$base/$duplex_box_id/box")"
test ! -e "$cg_root/box-$duplex_box_id"
vertex-sandbox cleanup --box-id "$duplex_box_id" --base "$base"
rm -f "$duplex_left_to_right" "$duplex_right_to_left"

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
grep -q '^termination-reason:time-limit$' "$base/$box_id/control/meta"
grep -q '^time-result:soft$' "$base/$box_id/control/meta"
grep -q '^time-limit:cpu$' "$base/$box_id/control/meta"

vertex-sandbox run \
  --box-id "$box_id" --base "$base" \
  --time-ms 100 --time-hard-ms 300 \
  --wall-ms 1000 --wall-hard-ms 2000 --memory-kb 262144 \
  --processes 8 --output-bytes 1048576 \
  --workspace-bytes 67108864 --workspace-inodes 4096 \
  -- /usr/bin/python3 -c 'while True: pass'
grep -q '^status:TO$' "$base/$box_id/control/meta"
grep -q '^killed:1$' "$base/$box_id/control/meta"
grep -q '^time-result:hard$' "$base/$box_id/control/meta"
grep -q '^time-limit:cpu$' "$base/$box_id/control/meta"

vertex-sandbox run \
  --box-id "$box_id" --base "$base" \
  --time-ms 2000 --time-hard-ms 3000 \
  --wall-ms 100 --wall-hard-ms 1000 --memory-kb 262144 \
  --processes 8 --output-bytes 1048576 \
  --workspace-bytes 67108864 --workspace-inodes 4096 \
  -- /usr/bin/python3 -c 'import time; time.sleep(0.25)'
grep -q '^status:TO$' "$base/$box_id/control/meta"
grep -q '^killed:0$' "$base/$box_id/control/meta"
grep -q '^time-result:soft$' "$base/$box_id/control/meta"
grep -q '^time-limit:wall$' "$base/$box_id/control/meta"

vertex-sandbox run \
  --box-id "$box_id" --base "$base" \
  --time-ms 2000 --time-hard-ms 3000 \
  --wall-ms 100 --wall-hard-ms 300 --memory-kb 262144 \
  --processes 8 --output-bytes 1048576 \
  --workspace-bytes 67108864 --workspace-inodes 4096 \
  -- /usr/bin/python3 -c 'import time; time.sleep(60)'
grep -q '^status:TO$' "$base/$box_id/control/meta"
grep -q '^killed:1$' "$base/$box_id/control/meta"
grep -q '^time-result:hard$' "$base/$box_id/control/meta"
grep -q '^time-limit:wall$' "$base/$box_id/control/meta"

vertex-sandbox run \
  --box-id "$box_id" --base "$base" \
  --time-ms 2000 --time-hard-ms 3000 \
  --wall-ms 4000 --wall-hard-ms 5000 --memory-kb 262144 \
  --processes 8 --output-bytes 65536 \
  --workspace-bytes 67108864 --workspace-inodes 4096 \
  -- /usr/bin/python3 -c 'import os; os.write(1, b"x" * 131072)'
grep -q '^exitsig:25$' "$base/$box_id/control/meta"
grep -q '^output-limit:1$' "$base/$box_id/control/meta"
grep -q '^termination-reason:output-limit$' "$base/$box_id/control/meta"
awk -F: '$1 == "stdout-bytes" && $2 >= 65536 { found=1 } END { exit !found }' \
  "$base/$box_id/control/meta"

vertex-sandbox run \
  --box-id "$box_id" --base "$base" \
  --time-ms 2000 --time-hard-ms 3000 \
  --wall-ms 4000 --wall-hard-ms 5000 --memory-kb 32768 \
  --processes 8 --output-bytes 1048576 \
  --workspace-bytes 67108864 --workspace-inodes 4096 \
  -- /usr/bin/python3 -c 'bytearray(256 * 1024 * 1024)'
grep -q '^cg-oom-killed:1$' "$base/$box_id/control/meta"
grep -q '^termination-reason:memory-limit$' "$base/$box_id/control/meta"

# The memory budget covers all children together, not only each process's RSS.
vertex-sandbox run \
  --box-id "$box_id" --base "$base" \
  --time-ms 2000 --time-hard-ms 3000 \
  --wall-ms 4000 --wall-hard-ms 5000 --memory-kb 65536 \
  --processes 8 --output-bytes 1048576 \
  --workspace-bytes 67108864 --workspace-inodes 4096 \
  -- /usr/bin/python3 -c 'import os, time
for _ in range(2):
    if os.fork() == 0:
        data = bytearray(48 * 1024 * 1024)
        time.sleep(2)
        os._exit(0)
for _ in range(2): os.wait()'
grep -q '^cg-oom-killed:1$' "$base/$box_id/control/meta"
test ! -e "$cg_root/box-$box_id"

# A bounded fork attempt must hit the task's pids.max. All sleeping children
# are reaped when their parent exits, without affecting the privileged worker.
vertex-sandbox run \
  --box-id "$box_id" --base "$base" \
  --time-ms 2000 --time-hard-ms 3000 \
  --wall-ms 4000 --wall-hard-ms 5000 --memory-kb 262144 \
  --processes 4 --output-bytes 1048576 \
  --workspace-bytes 67108864 --workspace-inodes 4096 \
  -- /usr/bin/python3 -c 'import errno, os, time
for _ in range(8):
    try:
        pid = os.fork()
    except OSError as error:
        assert error.errno == errno.EAGAIN
        print("pids-limited", flush=True)
        os._exit(0)
    if pid == 0:
        time.sleep(60)
        os._exit(0)
raise AssertionError("process limit was not enforced")'
test "$(cat "$base/$box_id/control/stdout")" = "pids-limited"
test ! -e "$cg_root/box-$box_id"

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
grep -q '^termination-reason:workspace-limit$' "$base/$box_id/control/meta"

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
