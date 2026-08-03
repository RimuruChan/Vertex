#pragma once

#include <vertex/sandbox.hpp>
#include <vertex/internal/options.hpp>

#include <cstdint>
#include <string>

#include <sys/resource.h>
#include <sys/types.h>

namespace vertex::sandbox::internal {

class Cgroup;
class SandboxPaths;

struct RunStats {
    int wait_status = 0;
    bool child_reaped = false;
    bool timed_out = false;
    bool hard_timed_out = false;
    bool cpu_timed_out = false;
    bool cpu_hard_timed_out = false;
    bool wall_timed_out = false;
    bool wall_hard_timed_out = false;
    bool output_exceeded = false;
    bool output_killed = false;
    bool timeout_killed = false;
    bool workspace_exceeded = false;
    bool workspace_killed = false;
    bool cancelled = false;
    bool oom_killed = false;
    std::uint64_t cpu_usec = 0;
    std::uint64_t wall_usec = 0;
    std::uint64_t memory_peak_bytes = 0;
    std::uint64_t stdout_bytes = 0;
    std::uint64_t stderr_bytes = 0;
    std::uint64_t workspace_bytes = 0;
    std::uint64_t workspace_inodes = 0;
    struct rusage usage {};
    std::string setup_error;
};

RunStats supervise(const Options& options, const SandboxPaths& paths,
                   Cgroup& cgroup, pid_t child, int error_fd,
                   int stdout_fd, int stderr_fd,
                   const CancellationToken* cancellation);
void write_meta(const Options& options, const SandboxPaths& paths,
                const RunStats& stats);

}  // namespace vertex::sandbox::internal
