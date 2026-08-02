// Vertex sandbox runner.
//
// The process supervision model is inspired by established contest runners
// such as DOMjudge runguard, but this is an independent implementation.  In
// particular, filesystem isolation uses Landlock instead of a mounted chroot,
// so running inside Docker does not require mount(2) or an unconfined AppArmor
// profile.

#include <algorithm>
#include <array>
#include <cerrno>
#include <charconv>
#include <chrono>
#include <csignal>
#include <cstring>
#include <filesystem>
#include <fstream>
#include <iostream>
#include <limits>
#include <optional>
#include <stdexcept>
#include <string>
#include <string_view>
#include <system_error>
#include <thread>
#include <vector>

#include <fcntl.h>
#include <grp.h>
#include <linux/capability.h>
#include <linux/landlock.h>
#include <linux/sched.h>
#include <seccomp.h>
#include <sys/prctl.h>
#include <sys/resource.h>
#include <sys/stat.h>
#include <sys/syscall.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <unistd.h>

extern char** environ;

namespace fs = std::filesystem;
using Clock = std::chrono::steady_clock;

namespace {

constexpr int kFirstRunUid = 60000;
constexpr int kMaxBoxId = 4095;
constexpr std::string_view kBaseMarker = ".vertex-sandbox-root";
constexpr std::chrono::milliseconds kPollInterval{5};
constexpr std::chrono::milliseconds kReapDeadline{500};

volatile sig_atomic_t received_signal = 0;

class Error : public std::runtime_error {
public:
    explicit Error(const std::string& message) : std::runtime_error(message) {}
};

class Fd {
public:
    explicit Fd(int value = -1) : value_(value) {}
    ~Fd() {
        if (value_ >= 0) {
            close(value_);
        }
    }
    Fd(const Fd&) = delete;
    Fd& operator=(const Fd&) = delete;
    Fd(Fd&& other) noexcept : value_(other.release()) {}
    Fd& operator=(Fd&& other) noexcept {
        if (this != &other) {
            if (value_ >= 0) {
                close(value_);
            }
            value_ = other.release();
        }
        return *this;
    }
    [[nodiscard]] int get() const { return value_; }
    [[nodiscard]] int release() {
        const int value = value_;
        value_ = -1;
        return value;
    }

private:
    int value_;
};

struct Options {
    std::string action;
    int box_id = -1;
    fs::path base = "/var/local/lib/vertex-sandbox";
    std::uint64_t time_ms = 0;
    std::uint64_t wall_ms = 0;
    std::uint64_t memory_kb = 0;
    std::uint64_t processes = 1;
    std::uint64_t output_bytes = 0;
    std::uint64_t stack_kb = 0;
    std::string stdin_name;
    std::vector<std::string> environment;
    std::vector<std::string> command;
};

struct RunStats {
    int wait_status = 0;
    bool child_reaped = false;
    bool timed_out = false;
    bool output_exceeded = false;
    bool cancelled = false;
    bool oom_killed = false;
    std::uint64_t cpu_usec = 0;
    std::uint64_t wall_usec = 0;
    std::uint64_t memory_peak_bytes = 0;
    struct rusage usage {};
    std::string setup_error;
};

[[noreturn]] void fail_errno(std::string_view operation) {
    throw Error(std::string(operation) + ": " + std::strerror(errno));
}

void signal_handler(int signal_number) {
    received_signal = signal_number;
}

std::uint64_t parse_uint(std::string_view value, std::string_view name) {
    std::uint64_t parsed = 0;
    const auto* begin = value.data();
    const auto* end = begin + value.size();
    const auto result = std::from_chars(begin, end, parsed);
    if (result.ec != std::errc{} || result.ptr != end) {
        throw Error("invalid " + std::string(name) + ": " + std::string(value));
    }
    return parsed;
}

std::uint64_t checked_multiply(std::uint64_t value, std::uint64_t factor,
                               std::string_view name) {
    if (value > std::numeric_limits<std::uint64_t>::max() / factor) {
        throw Error(std::string(name) + " is too large");
    }
    return value * factor;
}

std::string require_value(int argc, char** argv, int& index, std::string_view option) {
    if (++index >= argc) {
        throw Error("missing value for " + std::string(option));
    }
    return argv[index];
}

Options parse_options(int argc, char** argv) {
    if (argc < 2) {
        throw Error("expected one of: probe, init, cleanup, run");
    }

    Options options;
    options.action = argv[1];
    if (options.action != "probe" && options.action != "init" &&
        options.action != "cleanup" && options.action != "run") {
        throw Error("unknown action: " + options.action);
    }

    for (int i = 2; i < argc; ++i) {
        const std::string_view arg = argv[i];
        if (arg == "--") {
            for (++i; i < argc; ++i) {
                options.command.emplace_back(argv[i]);
            }
            break;
        }
        if (arg == "--box-id") {
            const auto value = require_value(argc, argv, i, arg);
            const auto parsed = parse_uint(value, "box id");
            if (parsed > static_cast<std::uint64_t>(kMaxBoxId)) {
                throw Error("box id out of range");
            }
            options.box_id = static_cast<int>(parsed);
        } else if (arg == "--base") {
            options.base = require_value(argc, argv, i, arg);
        } else if (arg == "--time-ms") {
            options.time_ms = parse_uint(require_value(argc, argv, i, arg), "time limit");
        } else if (arg == "--wall-ms") {
            options.wall_ms = parse_uint(require_value(argc, argv, i, arg), "wall limit");
        } else if (arg == "--memory-kb") {
            options.memory_kb = parse_uint(require_value(argc, argv, i, arg), "memory limit");
        } else if (arg == "--processes") {
            options.processes = parse_uint(require_value(argc, argv, i, arg), "process limit");
        } else if (arg == "--output-bytes") {
            options.output_bytes = parse_uint(require_value(argc, argv, i, arg), "output limit");
        } else if (arg == "--stack-kb") {
            options.stack_kb = parse_uint(require_value(argc, argv, i, arg), "stack limit");
        } else if (arg == "--stdin") {
            options.stdin_name = require_value(argc, argv, i, arg);
        } else if (arg == "--env") {
            options.environment.push_back(require_value(argc, argv, i, arg));
        } else {
            throw Error("unknown option: " + std::string(arg));
        }
    }

    if (options.action != "probe" && options.box_id < 0) {
        throw Error("--box-id is required");
    }
    if (options.action == "run") {
        if (options.command.empty()) {
            throw Error("run requires a command after --");
        }
        if (options.time_ms == 0 || options.wall_ms == 0 || options.memory_kb == 0 ||
            options.processes == 0 || options.output_bytes == 0) {
            throw Error("run limits must all be positive");
        }
        // Validate every conversion before creating a cgroup or forking.
        (void)checked_multiply(options.time_ms, 1000, "time limit");
        (void)checked_multiply(options.wall_ms, 1000, "wall limit");
        (void)checked_multiply(options.memory_kb, 1024, "memory limit");
        if (options.stack_kb > 0) {
            (void)checked_multiply(options.stack_kb, 1024, "stack limit");
        }
    }
    return options;
}

void validate_base(const fs::path& base) {
    if (!base.is_absolute() || base == "/" || base.lexically_normal() != base) {
        throw Error("sandbox base must be a normalized absolute directory");
    }
    std::size_t components = 0;
    for ([[maybe_unused]] const auto& part : base) {
        ++components;
    }
    if (components < 3) {
        throw Error("sandbox base is too broad");
    }
}

void validate_relative_name(const std::string& name) {
    if (name.empty()) {
        return;
    }
    const fs::path path{name};
    if (path.is_absolute() || path.lexically_normal() != path || path.has_parent_path() ||
        name == "." || name == ".." || name.find('\\') != std::string::npos) {
        throw Error("sandbox file name must be one relative path component: " + name);
    }
}

fs::path box_root(const Options& options) {
    validate_base(options.base);
    return options.base / std::to_string(options.box_id);
}

fs::path work_dir(const Options& options) {
    return box_root(options) / "box";
}

fs::path control_dir(const Options& options) {
    return box_root(options) / "control";
}

fs::path cgroup_root() {
    if (const char* configured = std::getenv("VERTEX_CGROUP_ROOT"); configured && *configured) {
        return configured;
    }
    return "/vertex-cgroup";
}

fs::path cgroup_dir(const Options& options) {
    return cgroup_root() / ("box-" + std::to_string(options.box_id));
}

void require_base_marker(const fs::path& base) {
    validate_base(base);
    const fs::path marker = base / kBaseMarker;
    struct stat status {};
    if (lstat(marker.c_str(), &status) != 0 || !S_ISREG(status.st_mode) ||
        status.st_uid != 0 || (status.st_mode & 0022) != 0) {
        throw Error("sandbox base is missing its trusted marker: " + base.string());
    }
}

void initialize_base(const fs::path& base) {
    validate_base(base);
    fs::create_directories(base);
    const fs::path marker = base / kBaseMarker;
    if (!fs::exists(marker)) {
        if (fs::directory_iterator(base) != fs::directory_iterator{}) {
            throw Error("refusing to initialize a non-empty sandbox base: " +
                        base.string());
        }
        const Fd fd(open(marker.c_str(), O_WRONLY | O_CREAT | O_EXCL | O_CLOEXEC |
                                             O_NOFOLLOW,
                         0600));
        if (fd.get() < 0 && errno != EEXIST) {
            fail_errno("create sandbox base marker");
        }
        if (fd.get() >= 0) {
            constexpr std::string_view contents = "vertex-sandbox-v1\n";
            if (write(fd.get(), contents.data(), contents.size()) !=
                static_cast<ssize_t>(contents.size())) {
                fail_errno("write sandbox base marker");
            }
        }
    }
    require_base_marker(base);
}

void write_text(const fs::path& path, std::string_view value) {
    std::ofstream output(path);
    if (!output) {
        throw Error("open " + path.string() + ": " + std::strerror(errno));
    }
    output << value;
    output.flush();
    if (!output) {
        throw Error("write " + path.string() + ": " + std::strerror(errno));
    }
}

std::optional<std::uint64_t> read_uint_file(const fs::path& path) {
    std::ifstream input(path);
    std::uint64_t value = 0;
    if (!(input >> value)) {
        return std::nullopt;
    }
    return value;
}

std::uint64_t read_keyed_uint(const fs::path& path, std::string_view wanted) {
    std::ifstream input(path);
    std::string key;
    std::uint64_t value = 0;
    while (input >> key >> value) {
        if (key == wanted) {
            return value;
        }
    }
    return 0;
}

int landlock_abi() {
    errno = 0;
    return static_cast<int>(syscall(SYS_landlock_create_ruleset, nullptr, 0,
                                    LANDLOCK_CREATE_RULESET_VERSION));
}

std::uint64_t landlock_access_mask(int abi) {
    std::uint64_t access =
        LANDLOCK_ACCESS_FS_EXECUTE |
        LANDLOCK_ACCESS_FS_WRITE_FILE |
        LANDLOCK_ACCESS_FS_READ_FILE |
        LANDLOCK_ACCESS_FS_READ_DIR |
        LANDLOCK_ACCESS_FS_REMOVE_DIR |
        LANDLOCK_ACCESS_FS_REMOVE_FILE |
        LANDLOCK_ACCESS_FS_MAKE_CHAR |
        LANDLOCK_ACCESS_FS_MAKE_DIR |
        LANDLOCK_ACCESS_FS_MAKE_REG |
        LANDLOCK_ACCESS_FS_MAKE_SOCK |
        LANDLOCK_ACCESS_FS_MAKE_FIFO |
        LANDLOCK_ACCESS_FS_MAKE_BLOCK |
        LANDLOCK_ACCESS_FS_MAKE_SYM;
#ifdef LANDLOCK_ACCESS_FS_REFER
    if (abi >= 2) {
        access |= LANDLOCK_ACCESS_FS_REFER;
    }
#endif
#ifdef LANDLOCK_ACCESS_FS_TRUNCATE
    if (abi >= 3) {
        access |= LANDLOCK_ACCESS_FS_TRUNCATE;
    }
#endif
#ifdef LANDLOCK_ACCESS_FS_IOCTL_DEV
    if (abi >= 5) {
        access |= LANDLOCK_ACCESS_FS_IOCTL_DEV;
    }
#endif
    return access;
}

void add_landlock_path(int ruleset_fd, const fs::path& path, std::uint64_t access) {
    const Fd path_fd(open(path.c_str(), O_PATH | O_CLOEXEC));
    if (path_fd.get() < 0) {
        if (errno == ENOENT) {
            return;
        }
        fail_errno("open Landlock path " + path.string());
    }
    landlock_path_beneath_attr rule{};
    rule.allowed_access = access;
    rule.parent_fd = path_fd.get();
    if (syscall(SYS_landlock_add_rule, ruleset_fd, LANDLOCK_RULE_PATH_BENEATH,
                &rule, 0) != 0) {
        fail_errno("add Landlock rule " + path.string());
    }
}

void apply_landlock(const fs::path& workspace) {
    const int abi = landlock_abi();
    if (abi < 1) {
        fail_errno("Landlock is required");
    }
    const std::uint64_t handled = landlock_access_mask(abi);
    landlock_ruleset_attr ruleset{};
    ruleset.handled_access_fs = handled;
    const Fd ruleset_fd(static_cast<int>(syscall(SYS_landlock_create_ruleset,
                                                   &ruleset, sizeof(ruleset), 0)));
    if (ruleset_fd.get() < 0) {
        fail_errno("create Landlock ruleset");
    }

    const std::uint64_t read_exec = LANDLOCK_ACCESS_FS_EXECUTE |
                                    LANDLOCK_ACCESS_FS_READ_FILE |
                                    LANDLOCK_ACCESS_FS_READ_DIR;
    const std::uint64_t read_only = LANDLOCK_ACCESS_FS_READ_FILE |
                                    LANDLOCK_ACCESS_FS_READ_DIR;
    std::uint64_t workspace_access = handled &
        ~(LANDLOCK_ACCESS_FS_MAKE_CHAR | LANDLOCK_ACCESS_FS_MAKE_BLOCK);
#ifdef LANDLOCK_ACCESS_FS_IOCTL_DEV
    workspace_access &= ~LANDLOCK_ACCESS_FS_IOCTL_DEV;
#endif

    add_landlock_path(ruleset_fd.get(), workspace, workspace_access);
    for (const fs::path& path : {fs::path("/usr"), fs::path("/bin"),
                                 fs::path("/lib"), fs::path("/lib64")}) {
        add_landlock_path(ruleset_fd.get(), path, read_exec);
    }
    add_landlock_path(ruleset_fd.get(), "/etc", read_only);
    add_landlock_path(ruleset_fd.get(), "/dev/null",
                      LANDLOCK_ACCESS_FS_READ_FILE | LANDLOCK_ACCESS_FS_WRITE_FILE);
    for (const fs::path& path : {fs::path("/dev/zero"), fs::path("/dev/urandom"),
                                 fs::path("/dev/random")}) {
        add_landlock_path(ruleset_fd.get(), path, LANDLOCK_ACCESS_FS_READ_FILE);
    }

    if (syscall(SYS_landlock_restrict_self, ruleset_fd.get(), 0) != 0) {
        fail_errno("enforce Landlock ruleset");
    }
}

void add_seccomp_errno(scmp_filter_ctx filter, std::string_view name, int error_number) {
    const int syscall_number = seccomp_syscall_resolve_name(std::string(name).c_str());
    if (syscall_number == __NR_SCMP_ERROR) {
        return;
    }
    const int result = seccomp_rule_add(filter, SCMP_ACT_ERRNO(error_number),
                                        syscall_number, 0);
    if (result != 0 && result != -EEXIST) {
        throw Error("add seccomp rule " + std::string(name) + ": " +
                    std::strerror(-result));
    }
}

void apply_seccomp() {
    scmp_filter_ctx raw_filter = seccomp_init(SCMP_ACT_ALLOW);
    if (raw_filter == nullptr) {
        throw Error("create seccomp filter");
    }
    struct FilterGuard {
        scmp_filter_ctx filter;
        ~FilterGuard() { seccomp_release(filter); }
    } guard{raw_filter};

    constexpr std::array<std::string_view, 45> denied = {
        "_sysctl", "acct", "add_key", "bpf", "delete_module",
        "fanotify_init", "finit_module", "fsconfig", "fsmount", "fsopen",
        "fspick", "init_module", "ioperm", "iopl", "io_uring_setup",
        "kcmp", "kexec_file_load", "kexec_load", "keyctl", "lookup_dcookie",
        "mount", "mount_setattr", "move_mount", "name_to_handle_at",
        "open_by_handle_at", "open_tree", "perf_event_open", "pidfd_getfd",
        "pivot_root", "process_madvise", "process_vm_readv", "process_vm_writev",
        "ptrace", "quotactl", "reboot", "request_key", "setns", "swapon",
        "swapoff", "syslog", "umount", "umount2", "unshare", "userfaultfd",
        "vhangup"
    };
    for (const auto name : denied) {
        add_seccomp_errno(raw_filter, name, EPERM);
    }

    for (const std::string_view name : {"socket", "socketpair", "connect", "bind",
                                        "listen", "accept", "accept4", "sendto",
                                        "sendmmsg", "recvmmsg"}) {
        add_seccomp_errno(raw_filter, name, EACCES);
    }
    // Returning ENOSYS lets modern libc transparently fall back to clone(2).
    add_seccomp_errno(raw_filter, "clone3", ENOSYS);

    const int clone_number = seccomp_syscall_resolve_name("clone");
    if (clone_number != __NR_SCMP_ERROR) {
        constexpr std::array<std::uint64_t, 7> namespace_flags = {
            CLONE_NEWCGROUP, CLONE_NEWIPC, CLONE_NEWNET, CLONE_NEWNS,
            CLONE_NEWPID, CLONE_NEWUSER, CLONE_NEWUTS
        };
        for (const auto flag : namespace_flags) {
            const int result = seccomp_rule_add(raw_filter, SCMP_ACT_ERRNO(EPERM),
                                                clone_number, 1,
                                                SCMP_A0(SCMP_CMP_MASKED_EQ, flag, flag));
            if (result != 0 && result != -EEXIST) {
                throw Error("add seccomp clone rule: " +
                            std::string(std::strerror(-result)));
            }
        }
    }

    const int result = seccomp_load(raw_filter);
    if (result != 0) {
        throw Error("load seccomp filter: " + std::string(std::strerror(-result)));
    }
}

void clear_capabilities() {
    __user_cap_header_struct header{};
    header.version = _LINUX_CAPABILITY_VERSION_3;
    header.pid = 0;
    std::array<__user_cap_data_struct, 2> data{};
    if (syscall(SYS_capset, &header, data.data()) != 0) {
        fail_errno("clear capabilities");
    }
    if (prctl(PR_CAP_AMBIENT, PR_CAP_AMBIENT_CLEAR_ALL, 0, 0, 0) != 0 &&
        errno != EINVAL) {
        fail_errno("clear ambient capabilities");
    }
}

void set_limit(int resource, rlim_t value, std::string_view name) {
    const rlimit limit{value, value};
    if (setrlimit(resource, &limit) != 0) {
        fail_errno("set " + std::string(name));
    }
}

void apply_resource_limits(const Options& options) {
    set_limit(RLIMIT_CORE, 0, "core limit");
    set_limit(RLIMIT_NOFILE, 64, "file descriptor limit");
    set_limit(RLIMIT_NPROC, static_cast<rlim_t>(options.processes), "process limit");
    set_limit(RLIMIT_FSIZE, static_cast<rlim_t>(options.output_bytes), "file limit");
    if (options.stack_kb > 0) {
        set_limit(RLIMIT_STACK,
                  static_cast<rlim_t>(checked_multiply(options.stack_kb, 1024,
                                                       "stack limit")),
                  "stack limit");
    }
    const auto cpu_seconds = static_cast<rlim_t>(options.time_ms / 1000 +
                                                  (options.time_ms % 1000 != 0) + 1);
    set_limit(RLIMIT_CPU, cpu_seconds, "CPU limit");
}

void close_fds_except(int preserved) {
    long maximum = sysconf(_SC_OPEN_MAX);
    if (maximum < 0 || maximum > 65536) {
        maximum = 65536;
    }
    for (int fd = 3; fd < maximum; ++fd) {
        if (fd != preserved) {
            close(fd);
        }
    }
}

[[noreturn]] void child_fail(int error_fd, std::string_view message) {
    const int saved_errno = errno;
    std::string output(message);
    if (saved_errno != 0) {
        output += ": ";
        output += std::strerror(saved_errno);
    }
    output.push_back('\n');
    const auto ignored = write(error_fd, output.data(), output.size());
    (void)ignored;
    _exit(125);
}

void configure_environment(const Options& options, const fs::path& workspace) {
    if (clearenv() != 0) {
        fail_errno("clear environment");
    }
    if (setenv("PATH", "/usr/bin:/bin", 1) != 0 || setenv("LANG", "C", 1) != 0 ||
        setenv("HOME", workspace.c_str(), 1) != 0 ||
        setenv("TMPDIR", workspace.c_str(), 1) != 0) {
        fail_errno("set base environment");
    }
    for (const auto& entry : options.environment) {
        const auto separator = entry.find('=');
        if (separator == std::string::npos || separator == 0) {
            throw Error("invalid environment entry");
        }
        const std::string name = entry.substr(0, separator);
        if (name == "PATH" || name == "GLIBC_TUNABLES" || name.starts_with("LD_") ||
            name.starts_with("DYLD_")) {
            throw Error("forbidden environment variable: " + name);
        }
        if (setenv(name.c_str(), entry.substr(separator + 1).c_str(), 1) != 0) {
            fail_errno("set environment " + name);
        }
    }
}

void execute_child(const Options& options, pid_t runner_pid, int sync_fd, int error_fd,
                   int stdin_fd, int stdout_fd, int stderr_fd) {
    if (prctl(PR_SET_PDEATHSIG, SIGKILL) != 0) {
        child_fail(error_fd, "set parent-death signal");
    }
    // Close the small race where the runner exits between fork() and prctl().
    if (getppid() != runner_pid) {
        child_fail(error_fd, "sandbox runner exited during setup");
    }
    char start = 0;
    if (read(sync_fd, &start, 1) != 1) {
        child_fail(error_fd, "wait for cgroup assignment");
    }
    close(sync_fd);

    if (dup2(stdin_fd, STDIN_FILENO) < 0 || dup2(stdout_fd, STDOUT_FILENO) < 0 ||
        dup2(stderr_fd, STDERR_FILENO) < 0) {
        child_fail(error_fd, "redirect standard streams");
    }
    if (error_fd != 3) {
        if (dup3(error_fd, 3, O_CLOEXEC) < 0) {
            child_fail(error_fd, "preserve setup error pipe");
        }
        close(error_fd);
        error_fd = 3;
    }
    close_fds_except(error_fd);

    try {
        if (setsid() < 0) {
            fail_errno("create process session");
        }
        umask(0077);
        apply_resource_limits(options);
        const fs::path workspace = work_dir(options);
        if (chdir(workspace.c_str()) != 0) {
            fail_errno("change to workspace");
        }
        configure_environment(options, workspace);
        if (prctl(PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0) != 0) {
            fail_errno("set no_new_privs");
        }
        apply_landlock(workspace);

        const gid_t gid = static_cast<gid_t>(kFirstRunUid + options.box_id);
        const uid_t uid = static_cast<uid_t>(kFirstRunUid + options.box_id);
        if (setgroups(0, nullptr) != 0 || setresgid(gid, gid, gid) != 0 ||
            setresuid(uid, uid, uid) != 0) {
            fail_errno("drop credentials");
        }
        clear_capabilities();
        apply_seccomp();

        std::vector<char*> arguments;
        arguments.reserve(options.command.size() + 1);
        for (const auto& argument : options.command) {
            arguments.push_back(const_cast<char*>(argument.c_str()));
        }
        arguments.push_back(nullptr);
        execve(arguments.front(), arguments.data(), ::environ);
        fail_errno("execute " + options.command.front());
    } catch (const std::exception& error) {
        errno = 0;
        child_fail(error_fd, error.what());
    }
}

void kill_cgroup(const fs::path& group) {
    std::error_code ignored;
    if (fs::exists(group / "cgroup.kill", ignored)) {
        try {
            write_text(group / "cgroup.kill", "1");
            return;
        } catch (const Error&) {
            // Fall back to enumerating cgroup.procs below.
        }
    }
    std::ifstream processes(group / "cgroup.procs");
    pid_t pid = 0;
    while (processes >> pid) {
        if (pid > 1) {
            kill(pid, SIGKILL);
        }
    }
}

bool cgroup_populated(const fs::path& group) {
    return read_keyed_uint(group / "cgroup.events", "populated") != 0;
}

void remove_cgroup(const fs::path& group) {
    std::error_code ignored;
    if (!fs::exists(group, ignored)) {
        return;
    }
    const auto deadline = Clock::now() + kReapDeadline;
    while (cgroup_populated(group) && Clock::now() < deadline) {
        // cgroup.kill is unavailable on some otherwise supported cgroup v2
        // kernels. Re-enumeration closes the fork race in that fallback path.
        kill_cgroup(group);
        std::this_thread::sleep_for(kPollInterval);
    }
    kill_cgroup(group);
    if (rmdir(group.c_str()) != 0 && errno != ENOENT) {
        throw Error("remove cgroup " + group.string() + ": " + std::strerror(errno));
    }
}

void create_cgroup(const Options& options) {
    const fs::path root = cgroup_root();
    if (!fs::exists(root / "cgroup.controllers")) {
        throw Error("writable delegated cgroup v2 root is required: " + root.string());
    }
    const fs::path group = cgroup_dir(options);
    remove_cgroup(group);
    if (!fs::create_directory(group)) {
        throw Error("create cgroup: " + group.string());
    }
    try {
        write_text(group / "memory.max",
                   std::to_string(checked_multiply(options.memory_kb, 1024,
                                                   "memory limit")));
        if (fs::exists(group / "memory.swap.max")) {
            write_text(group / "memory.swap.max", "0");
        }
        if (fs::exists(group / "memory.oom.group")) {
            write_text(group / "memory.oom.group", "1");
        }
        write_text(group / "pids.max", std::to_string(options.processes));
    } catch (...) {
        try {
            remove_cgroup(group);
        } catch (...) {
        }
        throw;
    }
}

void update_cgroup_stats(const fs::path& group, RunStats& stats) {
    stats.cpu_usec = std::max(stats.cpu_usec,
                              read_keyed_uint(group / "cpu.stat", "usage_usec"));
    if (const auto peak = read_uint_file(group / "memory.peak")) {
        stats.memory_peak_bytes = std::max(stats.memory_peak_bytes, *peak);
    } else if (const auto current = read_uint_file(group / "memory.current")) {
        stats.memory_peak_bytes = std::max(stats.memory_peak_bytes, *current);
    }
    stats.oom_killed = read_keyed_uint(group / "memory.events", "oom_kill") > 0;
}

bool file_at_limit(int fd, std::uint64_t limit) {
    struct stat status {};
    return fstat(fd, &status) == 0 && status.st_size >= 0 &&
           static_cast<std::uint64_t>(status.st_size) >= limit;
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

RunStats supervise(const Options& options, pid_t child, int error_fd,
                   int stdout_fd, int stderr_fd) {
    RunStats stats;
    const fs::path group = cgroup_dir(options);
    const auto start = Clock::now();
    const auto cpu_limit_usec = checked_multiply(options.time_ms, 1000, "time limit");
    const auto wall_limit_usec = checked_multiply(options.wall_ms, 1000, "wall limit");

    while (!stats.child_reaped) {
        const auto elapsed = std::chrono::duration_cast<std::chrono::microseconds>(
            Clock::now() - start);
        stats.wall_usec = static_cast<std::uint64_t>(elapsed.count());
        update_cgroup_stats(group, stats);

        if (!stats.timed_out &&
            (stats.cpu_usec >= cpu_limit_usec || stats.wall_usec >= wall_limit_usec)) {
            stats.timed_out = true;
            kill_cgroup(group);
        }
        if (!stats.output_exceeded &&
            (file_at_limit(stdout_fd, options.output_bytes) ||
             file_at_limit(stderr_fd, options.output_bytes))) {
            stats.output_exceeded = true;
            kill_cgroup(group);
        }
        if (received_signal != 0 && !stats.cancelled) {
            stats.cancelled = true;
            kill_cgroup(group);
        }

        const pid_t result = wait4(child, &stats.wait_status, WNOHANG, &stats.usage);
        if (result == child) {
            stats.child_reaped = true;
            break;
        }
        if (result < 0 && errno != EINTR) {
            fail_errno("wait for sandboxed process");
        }
        std::this_thread::sleep_for(kPollInterval);
    }

    kill_cgroup(group);
    reap_descendants();
    update_cgroup_stats(group, stats);
    stats.wall_usec = static_cast<std::uint64_t>(
        std::chrono::duration_cast<std::chrono::microseconds>(Clock::now() - start).count());
    stats.setup_error = read_all(error_fd);
    return stats;
}

void write_meta(const Options& options, const RunStats& stats) {
    const fs::path directory = control_dir(options);
    const fs::path temporary = directory / "meta.tmp";
    const fs::path destination = directory / "meta";
    std::ofstream output(temporary, std::ios::trunc);
    if (!output) {
        throw Error("create meta file");
    }

    std::string status;
    std::string message;
    int exit_code = 0;
    int exit_signal = 0;
    bool killed = false;
    if (!stats.setup_error.empty()) {
        status = "XX";
        message = stats.setup_error;
    } else if (stats.oom_killed) {
        status = "SG";
        exit_signal = SIGKILL;
        killed = true;
        message = "memory limit exceeded";
    } else if (stats.output_exceeded) {
        status = "SG";
        exit_signal = SIGXFSZ;
        killed = true;
        message = "output limit exceeded";
    } else if (stats.timed_out) {
        status = "TO";
        exit_signal = SIGKILL;
        killed = true;
        message = "time limit exceeded";
    } else if (stats.cancelled) {
        status = "XX";
        killed = true;
        message = "sandbox runner interrupted";
    } else if (WIFEXITED(stats.wait_status)) {
        exit_code = WEXITSTATUS(stats.wait_status);
        if (exit_code != 0) {
            status = "RE";
        }
    } else if (WIFSIGNALED(stats.wait_status)) {
        status = "SG";
        exit_signal = WTERMSIG(stats.wait_status);
    } else {
        status = "XX";
        message = "unknown child status";
    }

    const double cpu_seconds = static_cast<double>(stats.cpu_usec) / 1'000'000.0;
    const double wall_seconds = static_cast<double>(stats.wall_usec) / 1'000'000.0;
    const auto memory_kb = stats.memory_peak_bytes / 1024;
    output.setf(std::ios::fixed);
    output.precision(6);
    if (!status.empty()) {
        output << "status:" << status << '\n';
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

Fd open_input(const Options& options) {
    if (options.stdin_name.empty()) {
        const int fd = open("/dev/null", O_RDONLY | O_CLOEXEC);
        if (fd < 0) {
            fail_errno("open /dev/null");
        }
        return Fd(fd);
    }
    validate_relative_name(options.stdin_name);
    const fs::path path = work_dir(options) / options.stdin_name;
    const int fd = open(path.c_str(), O_RDONLY | O_CLOEXEC | O_NOFOLLOW);
    if (fd < 0) {
        fail_errno("open stdin " + path.string());
    }
    struct stat status {};
    if (fstat(fd, &status) != 0 || !S_ISREG(status.st_mode)) {
        close(fd);
        throw Error("stdin must be a regular file");
    }
    return Fd(fd);
}

Fd open_control_output(const Options& options, std::string_view name) {
    const fs::path path = control_dir(options) / name;
    const int fd = open(path.c_str(), O_WRONLY | O_CREAT | O_TRUNC | O_CLOEXEC |
                                      O_NOFOLLOW, 0644);
    if (fd < 0) {
        fail_errno("open output " + path.string());
    }
    return Fd(fd);
}

void initialize_box(const Options& options) {
    if (landlock_abi() < 1) {
        fail_errno("Landlock is required");
    }
    initialize_base(options.base);
    // Init is safe as a standalone recovery operation too: never replace a
    // workspace while descendants from an earlier runner may still use it.
    remove_cgroup(cgroup_dir(options));
    const fs::path root = box_root(options);
    std::error_code error;
    fs::remove_all(root, error);
    if (error) {
        throw Error("remove stale box " + root.string() + ": " + error.message());
    }
    fs::create_directories(work_dir(options));
    fs::create_directory(control_dir(options));
    if (chmod(root.c_str(), 0755) != 0 || chmod(work_dir(options).c_str(), 0700) != 0 ||
        chmod(control_dir(options).c_str(), 0700) != 0) {
        fail_errno("set box permissions");
    }
    const uid_t uid = static_cast<uid_t>(kFirstRunUid + options.box_id);
    const gid_t gid = static_cast<gid_t>(kFirstRunUid + options.box_id);
    if (chown(work_dir(options).c_str(), uid, gid) != 0) {
        fail_errno("set box ownership");
    }
}

void cleanup_box(const Options& options) {
    require_base_marker(options.base);
    remove_cgroup(cgroup_dir(options));
    const fs::path root = box_root(options);
    std::error_code error;
    fs::remove_all(root, error);
    if (error) {
        throw Error("remove box " + root.string() + ": " + error.message());
    }
}

void run_command(const Options& options) {
    require_base_marker(options.base);
    validate_relative_name(options.stdin_name);
    if (!fs::is_directory(work_dir(options)) || !fs::is_directory(control_dir(options))) {
        throw Error("box is not initialized");
    }
    if (options.command.front().find('/') == std::string::npos) {
        throw Error("command must be an absolute or explicit relative path");
    }

    Fd stdin_fd = open_input(options);
    Fd stdout_fd = open_control_output(options, "stdout");
    Fd stderr_fd = open_control_output(options, "stderr");
    std::error_code ignored;
    fs::remove(control_dir(options) / "meta", ignored);
    fs::remove(control_dir(options) / "meta.tmp", ignored);

    create_cgroup(options);
    struct CgroupGuard {
        fs::path path;
        ~CgroupGuard() {
            try {
                remove_cgroup(path);
            } catch (...) {
            }
        }
    } cgroup_guard{cgroup_dir(options)};

    std::array<int, 2> sync_pipe{};
    std::array<int, 2> error_pipe{};
    if (pipe2(sync_pipe.data(), O_CLOEXEC) != 0 || pipe2(error_pipe.data(), O_CLOEXEC) != 0) {
        fail_errno("create synchronization pipes");
    }
    Fd sync_read(sync_pipe[0]);
    Fd sync_write(sync_pipe[1]);
    Fd error_read(error_pipe[0]);
    Fd error_write(error_pipe[1]);

    if (prctl(PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0) != 0) {
        fail_errno("become child subreaper");
    }
    const pid_t runner_pid = getpid();
    const pid_t child = fork();
    if (child < 0) {
        fail_errno("fork sandboxed process");
    }
    if (child == 0) {
        sync_write = Fd{};
        error_read = Fd{};
        execute_child(options, runner_pid, sync_read.get(), error_write.get(), stdin_fd.get(),
                      stdout_fd.get(), stderr_fd.get());
    }

    sync_read = Fd{};
    error_write = Fd{};
    try {
        write_text(cgroup_dir(options) / "cgroup.procs", std::to_string(child));
        const char start = 1;
        if (write(sync_write.get(), &start, 1) != 1) {
            fail_errno("start sandboxed process");
        }
        sync_write = Fd{};
        const RunStats stats = supervise(options, child, error_read.get(),
                                         stdout_fd.get(), stderr_fd.get());
        write_meta(options, stats);
    } catch (...) {
        kill_cgroup(cgroup_dir(options));
        kill(child, SIGKILL);
        int status = 0;
        while (waitpid(child, &status, 0) < 0 && errno == EINTR) {
        }
        throw;
    }
}

void probe() {
    const int abi = landlock_abi();
    if (abi < 1) {
        fail_errno("Landlock is required");
    }
    const fs::path root = cgroup_root();
    const fs::path controllers = root / "cgroup.controllers";
    if (!fs::is_directory(root) || !fs::exists(controllers) ||
        access(root.c_str(), W_OK) != 0) {
        throw Error("cgroup v2 root is unavailable: " + cgroup_root().string());
    }
    std::ifstream input(controllers);
    std::string available;
    std::string controller;
    while (input >> controller) {
        available += " " + controller + " ";
    }
    for (const std::string_view required : {"cpu", "memory", "pids"}) {
        if (available.find(" " + std::string(required) + " ") == std::string::npos) {
            throw Error("required cgroup controller is unavailable: " +
                        std::string(required));
        }
    }
    std::cout << "landlock-abi=" << abi << " cgroup-v2=" << cgroup_root() << '\n';
}

}  // namespace

int main(int argc, char** argv) {
    try {
        const Options options = parse_options(argc, argv);
        struct sigaction action {};
        action.sa_handler = signal_handler;
        sigemptyset(&action.sa_mask);
        for (const int signal_number : {SIGINT, SIGTERM, SIGHUP}) {
            if (sigaction(signal_number, &action, nullptr) != 0) {
                fail_errno("install signal handler");
            }
        }

        if (options.action == "probe") {
            probe();
        } else if (options.action == "init") {
            initialize_box(options);
        } else if (options.action == "cleanup") {
            cleanup_box(options);
        } else {
            run_command(options);
        }
        return 0;
    } catch (const std::exception& error) {
        std::cerr << "vertex-sandbox: " << error.what() << '\n';
        return 2;
    }
}
