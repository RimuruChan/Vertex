#include <vertex/internal/security.hpp>

#include <vertex/internal/box.hpp>
#include <vertex/internal/constants.hpp>
#include <vertex/internal/error.hpp>
#include <vertex/internal/fd.hpp>

#include <array>
#include <cerrno>
#include <cstring>

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

extern char** environ;

namespace vertex::sandbox::internal {
namespace {

std::uint64_t landlock_access_mask(int abi) {
    (void)abi;
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
    const auto cpu_seconds = static_cast<rlim_t>(options.time_hard_ms / 1000 +
                                                   (options.time_hard_ms % 1000 != 0) + 1);
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

}  // namespace

int landlock_abi() {
    errno = 0;
    return static_cast<int>(syscall(SYS_landlock_create_ruleset, nullptr, 0,
                                    LANDLOCK_CREATE_RULESET_VERSION));
}

[[noreturn]] void execute_child(const Options& options, pid_t runner_pid,
                                int sync_fd, int error_fd, int stdin_fd,
                                int stdout_fd, int stderr_fd) {
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
        const fs::path workspace = SandboxPaths(options).workspace();
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

}  // namespace vertex::sandbox::internal
