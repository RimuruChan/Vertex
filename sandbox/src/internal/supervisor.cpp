#include <vertex/internal/supervisor.hpp>

#include <vertex/internal/box.hpp>
#include <vertex/internal/cgroup.hpp>
#include <vertex/internal/constants.hpp>
#include <vertex/internal/error.hpp>
#include <vertex/internal/fd.hpp>

#include <algorithm>
#include <array>
#include <cerrno>
#include <fstream>
#include <limits>
#include <thread>

#include <dirent.h>
#include <fcntl.h>
#include <sys/stat.h>
#include <sys/wait.h>

namespace vertex::sandbox::internal {
namespace {

struct WorkspaceUsage {
    std::uint64_t bytes = 0;
    std::uint64_t inodes = 0;
};

void saturating_add(std::uint64_t& target, std::uint64_t value) {
    if (value > std::numeric_limits<std::uint64_t>::max() - target) {
        target = std::numeric_limits<std::uint64_t>::max();
    } else {
        target += value;
    }
}

bool transient_workspace_error(int error_number) {
    return error_number == ENOENT || error_number == ENOTDIR ||
           error_number == ELOOP || error_number == ESTALE;
}

bool workspace_at_limit(const Options& options, const WorkspaceUsage& usage) {
    return usage.bytes >= options.workspace_bytes ||
           usage.inodes >= options.workspace_inodes;
}

void scan_workspace_directory(int directory_fd, const Options& options,
                              WorkspaceUsage& usage) {
    const int duplicate = dup(directory_fd);
    if (duplicate < 0) {
        fail_errno("duplicate workspace directory");
    }
    DIR* raw_directory = fdopendir(duplicate);
    if (raw_directory == nullptr) {
        const int saved_errno = errno;
        close(duplicate);
        errno = saved_errno;
        fail_errno("open workspace directory stream");
    }
    struct DirectoryGuard {
        DIR* value;
        ~DirectoryGuard() { closedir(value); }
    } guard{raw_directory};

    while (!workspace_at_limit(options, usage)) {
        errno = 0;
        dirent* entry = readdir(raw_directory);
        if (entry == nullptr) {
            if (errno != 0 && !transient_workspace_error(errno)) {
                fail_errno("read workspace directory");
            }
            return;
        }
        const std::string_view name{entry->d_name};
        if (name == "." || name == "..") {
            continue;
        }

        struct stat status {};
        if (fstatat(directory_fd, entry->d_name, &status, AT_SYMLINK_NOFOLLOW) != 0) {
            if (transient_workspace_error(errno)) {
                continue;
            }
            fail_errno("inspect workspace entry");
        }
        saturating_add(usage.inodes, 1);
        if (!S_ISDIR(status.st_mode) && status.st_size > 0) {
            saturating_add(usage.bytes, static_cast<std::uint64_t>(status.st_size));
        }
        if (!S_ISDIR(status.st_mode) || workspace_at_limit(options, usage)) {
            continue;
        }

        const Fd child_directory(openat(directory_fd, entry->d_name,
                                        O_RDONLY | O_DIRECTORY | O_CLOEXEC | O_NOFOLLOW));
        if (child_directory.get() < 0) {
            if (transient_workspace_error(errno)) {
                continue;
            }
            fail_errno("open workspace subdirectory");
        }
        scan_workspace_directory(child_directory.get(), options, usage);
    }
}

WorkspaceUsage scan_workspace(const Options& options, const SandboxPaths& paths) {
    const Fd root(open(paths.workspace().c_str(),
                       O_RDONLY | O_DIRECTORY | O_CLOEXEC | O_NOFOLLOW));
    if (root.get() < 0) {
        fail_errno("open workspace for accounting");
    }
    WorkspaceUsage usage;
    scan_workspace_directory(root.get(), options, usage);
    return usage;
}

std::uint64_t file_size(int fd) {
    struct stat status {};
    if (fstat(fd, &status) != 0 || status.st_size < 0) {
        return 0;
    }
    return static_cast<std::uint64_t>(status.st_size);
}

std::string read_all(int fd) {
    std::string result;
    std::array<char, 1024> buffer{};
    while (true) {
        const ssize_t count = read(fd, buffer.data(), buffer.size());
        if (count > 0) {
            result.append(buffer.data(), static_cast<std::size_t>(count));
        } else if (count == 0) {
            break;
        } else if (errno != EINTR) {
            break;
        }
    }
    return result;
}

void reap_descendants() {
    const auto deadline = Clock::now() + kReapDeadline;
    while (Clock::now() < deadline) {
        int status = 0;
        const pid_t result = waitpid(-1, &status, WNOHANG);
        if (result > 0) {
            continue;
        }
        if (result < 0 && errno == ECHILD) {
            return;
        }
        std::this_thread::sleep_for(kPollInterval);
    }
}

void update_time_stats(RunStats& stats, std::uint64_t cpu_soft_usec,
                       std::uint64_t cpu_hard_usec,
                       std::uint64_t wall_soft_usec, std::uint64_t wall_hard_usec) {
    stats.cpu_timed_out = stats.cpu_timed_out || stats.cpu_usec >= cpu_soft_usec;
    stats.wall_timed_out = stats.wall_timed_out || stats.wall_usec >= wall_soft_usec;
    stats.timed_out = stats.cpu_timed_out || stats.wall_timed_out;
    stats.cpu_hard_timed_out =
        stats.cpu_hard_timed_out || stats.cpu_usec >= cpu_hard_usec;
    stats.wall_hard_timed_out =
        stats.wall_hard_timed_out || stats.wall_usec >= wall_hard_usec;
}

}  // namespace

bool update_workspace_stats(const Options& options, const SandboxPaths& paths,
                            RunStats& stats) {
    const WorkspaceUsage usage = scan_workspace(options, paths);
    stats.workspace_bytes = std::max(stats.workspace_bytes, usage.bytes);
    stats.workspace_inodes = std::max(stats.workspace_inodes, usage.inodes);
    if (workspace_at_limit(options, usage)) {
        stats.workspace_exceeded = true;
    }
    return stats.workspace_exceeded;
}

bool update_output_stats(const Options& options, RunStats& stats,
                         int stdout_fd, int stderr_fd) {
    stats.stdout_bytes = std::max(stats.stdout_bytes, file_size(stdout_fd));
    stats.stderr_bytes = std::max(stats.stderr_bytes, file_size(stderr_fd));
    if (stats.stdout_bytes >= options.output_bytes ||
        stats.stderr_bytes >= options.output_bytes) {
        stats.output_exceeded = true;
    }
    return stats.output_exceeded;
}

RunStats supervise(const Options& options, const SandboxPaths& paths,
                   Cgroup& cgroup, pid_t child, int error_fd,
                   int stdout_fd, int stderr_fd,
                   const CancellationToken* cancellation) {
    RunStats stats;
    const auto start = Clock::now();
    const auto cpu_soft_usec = checked_multiply(options.time_ms, 1000, "time limit");
    const auto cpu_hard_usec =
        checked_multiply(options.time_hard_ms, 1000, "hard time limit");
    const auto wall_soft_usec = checked_multiply(options.wall_ms, 1000, "wall limit");
    const auto wall_hard_usec =
        checked_multiply(options.wall_hard_ms, 1000, "hard wall limit");

    while (!stats.child_reaped) {
        const auto elapsed = std::chrono::duration_cast<std::chrono::microseconds>(
            Clock::now() - start);
        stats.wall_usec = static_cast<std::uint64_t>(elapsed.count());
        cgroup.update_stats(stats);
        if (update_workspace_stats(options, paths, stats) && !stats.workspace_killed) {
            stats.workspace_killed = true;
            cgroup.kill_all();
        }

        update_time_stats(stats, cpu_soft_usec, cpu_hard_usec,
                          wall_soft_usec, wall_hard_usec);
        if (!stats.hard_timed_out &&
            (stats.cpu_hard_timed_out || stats.wall_hard_timed_out)) {
            stats.hard_timed_out = true;
            stats.timeout_killed = true;
            cgroup.kill_all();
        }
        const bool output_already_exceeded = stats.output_exceeded;
        if (update_output_stats(options, stats, stdout_fd, stderr_fd) &&
            !output_already_exceeded) {
            stats.output_killed = true;
            cgroup.kill_all();
        }
        if (cancellation != nullptr && cancellation->requested() &&
            !stats.cancelled) {
            stats.cancelled = true;
            cgroup.kill_all();
        }

        const pid_t result = wait4(child, &stats.wait_status, WNOHANG, &stats.usage);
        if (result == child) {
            stats.child_reaped = true;
            stats.wall_usec = static_cast<std::uint64_t>(
                std::chrono::duration_cast<std::chrono::microseconds>(Clock::now() - start)
                    .count());
            cgroup.update_stats(stats);
            (void)update_workspace_stats(options, paths, stats);
            (void)update_output_stats(options, stats, stdout_fd, stderr_fd);
            update_time_stats(stats, cpu_soft_usec, cpu_hard_usec,
                              wall_soft_usec, wall_hard_usec);
            stats.hard_timed_out =
                stats.cpu_hard_timed_out || stats.wall_hard_timed_out;
            break;
        }
        if (result < 0 && errno != EINTR) {
            fail_errno("wait for sandboxed process");
        }
        std::this_thread::sleep_for(kPollInterval);
    }

    cgroup.kill_all();
    reap_descendants();
    cgroup.update_stats(stats);
    (void)update_output_stats(options, stats, stdout_fd, stderr_fd);
    stats.setup_error = read_all(error_fd);
    return stats;
}

void write_meta(const Options& options, const SandboxPaths& paths,
                const RunStats& stats) {
    const fs::path temporary = paths.control() / "meta.tmp";
    const fs::path destination = paths.control() / "meta";
    std::ofstream output(temporary, std::ios::trunc);
    if (!output) {
        throw Error("create meta file");
    }

    std::string status;
    std::string message;
    std::string termination_reason;
    int exit_code = 0;
    int exit_signal = 0;
    bool killed = false;
    if (!stats.setup_error.empty()) {
        status = "XX";
        termination_reason = "setup-error";
        message = stats.setup_error;
    } else if (stats.oom_killed) {
        status = "SG";
        termination_reason = "memory-limit";
        exit_signal = SIGKILL;
        killed = true;
        message = "memory limit exceeded";
    } else if (stats.output_exceeded) {
        status = "SG";
        termination_reason = "output-limit";
        exit_signal = SIGXFSZ;
        killed = stats.output_killed;
        message = "output limit exceeded";
    } else if (stats.workspace_exceeded) {
        status = "SG";
        termination_reason = "workspace-limit";
        if (stats.workspace_killed) {
            exit_signal = SIGKILL;
            killed = true;
        }
        message = "workspace byte or inode limit exceeded";
    } else if (stats.timed_out) {
        status = "TO";
        termination_reason = "time-limit";
        if (stats.timeout_killed) {
            exit_signal = SIGKILL;
            killed = true;
        }
        if (stats.cpu_timed_out && stats.wall_timed_out) {
            message = "CPU and wall time limits exceeded";
        } else if (stats.cpu_timed_out) {
            message = "CPU time limit exceeded";
        } else {
            message = "wall time limit exceeded";
        }
    } else if (stats.cancelled) {
        status = "XX";
        termination_reason = "cancelled";
        killed = true;
        message = "sandbox runner interrupted";
    } else if (WIFEXITED(stats.wait_status)) {
        termination_reason = "exited";
        exit_code = WEXITSTATUS(stats.wait_status);
        if (exit_code != 0) {
            status = "RE";
        }
    } else if (WIFSIGNALED(stats.wait_status)) {
        status = "SG";
        termination_reason = "signal";
        exit_signal = WTERMSIG(stats.wait_status);
    } else {
        status = "XX";
        termination_reason = "setup-error";
        message = "unknown child status";
    }

    const std::string time_result = stats.hard_timed_out ? "hard" :
                                    stats.timed_out ? "soft" : "none";
    std::string time_limit;
    if (stats.cpu_timed_out) {
        time_limit = "cpu";
    }
    if (stats.wall_timed_out) {
        if (!time_limit.empty()) {
            time_limit += ',';
        }
        time_limit += "wall";
    }

    const double cpu_seconds = static_cast<double>(stats.cpu_usec) / 1'000'000.0;
    const double wall_seconds = static_cast<double>(stats.wall_usec) / 1'000'000.0;
    const auto memory_kb = stats.memory_peak_bytes / 1024;
    output.setf(std::ios::fixed);
    output.precision(6);
    if (!status.empty()) {
        output << "status:" << status << '\n';
    }
    output << "termination-reason:" << termination_reason << '\n';
    output << "time-result:" << time_result << '\n';
    if (!time_limit.empty()) {
        output << "time-limit:" << time_limit << '\n';
    }
    output << "time:" << cpu_seconds << '\n';
    output << "time-wall:" << wall_seconds << '\n';
    output << "max-rss:" << stats.usage.ru_maxrss << '\n';
    output << "cg-mem:" << memory_kb << '\n';
    output << "exitcode:" << exit_code << '\n';
    if (exit_signal != 0) {
        output << "exitsig:" << exit_signal << '\n';
    }
    output << "killed:" << (killed ? 1 : 0) << '\n';
    output << "cg-oom-killed:" << (stats.oom_killed ? 1 : 0) << '\n';
    output << "output-limit:" << (stats.output_exceeded ? 1 : 0) << '\n';
    output << "stdout-streamed:" << (options.stdout_fd >= 0 ? 1 : 0) << '\n';
    output << "stdout-bytes:" << stats.stdout_bytes << '\n';
    output << "stderr-bytes:" << stats.stderr_bytes << '\n';
    output << "workspace-limit:" << (stats.workspace_exceeded ? 1 : 0) << '\n';
    output << "workspace-bytes:" << stats.workspace_bytes << '\n';
    output << "workspace-inodes:" << stats.workspace_inodes << '\n';
    if (!message.empty()) {
        message.erase(std::remove(message.begin(), message.end(), '\n'), message.end());
        output << "message:" << message << '\n';
    }
    output.close();
    if (!output) {
        throw Error("write meta file");
    }
    fs::rename(temporary, destination);
}

}  // namespace vertex::sandbox::internal
