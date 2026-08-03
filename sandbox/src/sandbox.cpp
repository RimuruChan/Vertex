#include <vertex/sandbox.hpp>

#include <vertex/internal/box.hpp>
#include <vertex/internal/cgroup.hpp>
#include <vertex/internal/constants.hpp>
#include <vertex/internal/error.hpp>
#include <vertex/internal/fd.hpp>
#include <vertex/internal/options.hpp>
#include <vertex/internal/security.hpp>
#include <vertex/internal/supervisor.hpp>

#include <array>
#include <cerrno>
#include <fstream>
#include <utility>

#include <fcntl.h>
#include <sys/prctl.h>
#include <sys/wait.h>
#include <unistd.h>

namespace vertex::sandbox {
namespace {

internal::Options make_options(const SandboxConfig& sandbox) {
    internal::Options options;
    options.box_id = sandbox.box_id;
    options.base = sandbox.base;
    options.cpu_set = sandbox.cpu_set;
    return options;
}

internal::Options make_options(const SandboxConfig& sandbox,
                               const RunOptions& run) {
    internal::Options options = make_options(sandbox);
    options.time_ms = run.time_ms;
    options.time_hard_ms = run.time_hard_ms;
    options.wall_ms = run.wall_ms;
    options.wall_hard_ms = run.wall_hard_ms;
    options.memory_kb = run.memory_kb;
    options.processes = run.processes;
    options.output_bytes = run.output_bytes;
    options.workspace_bytes = run.workspace_bytes;
    options.workspace_inodes = run.workspace_inodes;
    options.stack_kb = run.stack_kb;
    options.stdin_name = run.stdin_name;
    options.stdin_fd = run.stdin_fd;
    options.stdout_fd = run.stdout_fd;
    options.environment = run.environment;
    options.command = run.command;
    return options;
}

void validate_config(const SandboxConfig& config) {
    if (config.box_id < 0) {
        throw internal::Error("--box-id is required");
    }
    if (config.box_id > internal::kMaxBoxId) {
        throw internal::Error("box id out of range");
    }
    for (const char character : config.cpu_set) {
        if ((character < '0' || character > '9') && character != ',' &&
            character != '-') {
            throw internal::Error("invalid CPU set: " + config.cpu_set);
        }
    }
}

void validate_run_options(internal::Options& options) {
    if (options.time_hard_ms == 0) {
        options.time_hard_ms = options.time_ms;
    }
    if (options.wall_hard_ms == 0) {
        options.wall_hard_ms = options.wall_ms;
    }
    if (options.command.empty()) {
        throw internal::Error("run requires a command after --");
    }
    if (options.time_ms == 0 || options.wall_ms == 0 || options.memory_kb == 0 ||
        options.processes == 0 || options.output_bytes == 0 ||
        options.workspace_bytes == 0 || options.workspace_inodes == 0) {
        throw internal::Error("run limits must all be positive");
    }
    if (options.time_hard_ms < options.time_ms ||
        options.wall_hard_ms < options.wall_ms) {
        throw internal::Error("hard time limits must not be below soft limits");
    }
    if (!options.stdin_name.empty() && options.stdin_fd >= 0) {
        throw internal::Error("--stdin and --stdin-fd are mutually exclusive");
    }
    if (options.stdin_fd >= 0 && options.stdin_fd == options.stdout_fd) {
        throw internal::Error("stdin and stdout descriptors must be distinct");
    }
    for (const auto& [descriptor, name] :
         {std::pair{options.stdin_fd, "stdin fd"},
          std::pair{options.stdout_fd, "stdout fd"}}) {
        if (descriptor >= 0 && descriptor <= STDERR_FILENO) {
            throw internal::Error(std::string(name) +
                                  " must be an inherited descriptor above 2");
        }
    }
    (void)internal::checked_multiply(options.time_ms, 1000, "time limit");
    (void)internal::checked_multiply(options.time_hard_ms, 1000,
                                     "hard time limit");
    (void)internal::checked_multiply(options.wall_ms, 1000, "wall limit");
    (void)internal::checked_multiply(options.wall_hard_ms, 1000,
                                     "hard wall limit");
    (void)internal::checked_multiply(options.memory_kb, 1024, "memory limit");
    if (options.stack_kb > 0) {
        (void)internal::checked_multiply(options.stack_kb, 1024, "stack limit");
    }
}

RunArtifacts run_command(const internal::Options& options,
                         const CancellationToken* cancellation) {
    const internal::SandboxPaths paths(options);
    paths.require_marker();
    internal::validate_relative_name(options.stdin_name);
    if (!internal::fs::is_directory(paths.workspace()) ||
        !internal::fs::is_directory(paths.control())) {
        throw internal::Error("box is not initialized");
    }
    if (options.command.front().find('/') == std::string::npos) {
        throw internal::Error("command must be an absolute or explicit relative path");
    }

    internal::Fd stdin_fd = internal::open_input(options, paths);
    if (options.stdout_fd >= 0) {
        std::error_code ignored;
        internal::fs::remove(paths.control() / "stdout", ignored);
    }
    internal::Fd stdout_fd = internal::open_control_output(options, paths, "stdout");
    internal::Fd stderr_fd = internal::open_control_output(options, paths, "stderr");
    std::error_code ignored;
    internal::fs::remove(paths.control() / "meta", ignored);
    internal::fs::remove(paths.control() / "meta.tmp", ignored);

    internal::Cgroup cgroup(options);
    cgroup.create();

    std::array<int, 2> sync_pipe{};
    std::array<int, 2> error_pipe{};
    if (pipe2(sync_pipe.data(), O_CLOEXEC) != 0 ||
        pipe2(error_pipe.data(), O_CLOEXEC) != 0) {
        internal::fail_errno("create synchronization pipes");
    }
    internal::Fd sync_read(sync_pipe[0]);
    internal::Fd sync_write(sync_pipe[1]);
    internal::Fd error_read(error_pipe[0]);
    internal::Fd error_write(error_pipe[1]);

    if (prctl(PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0) != 0) {
        internal::fail_errno("become child subreaper");
    }
    const pid_t runner_pid = getpid();
    const pid_t child = fork();
    if (child < 0) {
        internal::fail_errno("fork sandboxed process");
    }
    if (child == 0) {
        sync_write = internal::Fd{};
        error_read = internal::Fd{};
        internal::execute_child(options, runner_pid, sync_read.get(), error_write.get(),
                                stdin_fd.get(), stdout_fd.get(), stderr_fd.get());
    }

    sync_read = internal::Fd{};
    error_write = internal::Fd{};
    try {
        cgroup.add_process(child);
        const char start = 1;
        if (write(sync_write.get(), &start, 1) != 1) {
            internal::fail_errno("start sandboxed process");
        }
        sync_write = internal::Fd{};
        const internal::RunStats stats = internal::supervise(
            options, paths, cgroup, child, error_read.get(), stdout_fd.get(),
            stderr_fd.get(), cancellation);
        internal::write_meta(options, paths, stats);
    } catch (...) {
        cgroup.kill_all();
        kill(child, SIGKILL);
        int status = 0;
        while (waitpid(child, &status, 0) < 0 && errno == EINTR) {
        }
        throw;
    }

    return {
        .meta = paths.control() / "meta",
        .stdout_file = paths.control() / "stdout",
        .stderr_file = paths.control() / "stderr",
    };
}

}  // namespace

void CancellationToken::request() noexcept {
    requested_ = 1;
}

bool CancellationToken::requested() const noexcept {
    return requested_ != 0;
}

Sandbox::Sandbox(SandboxConfig config) : config_(std::move(config)) {
    validate_config(config_);
}

Capabilities Sandbox::probe() {
    const int abi = internal::landlock_abi();
    if (abi < 1) {
        internal::fail_errno("Landlock is required");
    }
    const internal::fs::path root = internal::Cgroup::root_path();
    const internal::fs::path controllers = root / "cgroup.controllers";
    if (!internal::fs::is_directory(root) || !internal::fs::exists(controllers) ||
        access(root.c_str(), W_OK) != 0) {
        throw internal::Error("cgroup v2 root is unavailable: " + root.string());
    }
    std::ifstream input(controllers);
    std::string available;
    std::string controller;
    while (input >> controller) {
        available += " " + controller + " ";
    }
    for (const std::string_view required : {"cpu", "memory", "pids"}) {
        if (available.find(" " + std::string(required) + " ") == std::string::npos) {
            throw internal::Error("required cgroup controller is unavailable: " +
                                  std::string(required));
        }
    }
    return {.landlock_abi = abi, .cgroup_root = root};
}

std::filesystem::path Sandbox::workspace_path() const {
    return internal::SandboxPaths(make_options(config_)).workspace();
}

void Sandbox::initialize() const {
    internal::initialize_box(make_options(config_));
}

void Sandbox::cleanup() const {
    internal::cleanup_box(make_options(config_));
}

RunArtifacts Sandbox::run(RunOptions run_options,
                          const CancellationToken* cancellation) const {
    internal::Options options = make_options(config_, run_options);
    validate_run_options(options);
    return run_command(options, cancellation);
}

}  // namespace vertex::sandbox
