#include <vertex/internal/environment.hpp>

#include <vertex/internal/box.hpp>
#include <vertex/internal/cgroup.hpp>
#include <vertex/internal/error.hpp>

#include <array>
#include <cerrno>
#include <cstring>
#include <fstream>
#include <iomanip>
#include <sstream>
#include <utility>

#include <fcntl.h>
#include <sys/file.h>
#include <sys/socket.h>
#include <sys/stat.h>

namespace vertex::sandbox::internal {
namespace {

constexpr const char* lease_root = "/vertex/run/sandbox";

fs::path lease_path(int slot) {
    return fs::path(lease_root) / ("slot-" + std::to_string(slot));
}

Fd open_lock(const fs::path& path) {
    Fd file(open(path.c_str(), O_RDWR | O_CREAT | O_CLOEXEC | O_NOFOLLOW, 0600));
    if (file.get() < 0) {
        fail_errno("open environment lease");
    }
    return file;
}

Options box_options(const SandboxConfig& config) {
    Options options;
    options.box_id = config.box_id;
    options.base = config.base;
    options.cpu_set = config.cpu_set;
    options.managed_environment = true;
    return options;
}

std::string read_record(int fd) {
    std::array<char, 8192> buffer{};
    const ssize_t count = pread(fd, buffer.data(), buffer.size(), 0);
    if (count < 0 || count == static_cast<ssize_t>(buffer.size())) {
        throw Error("invalid environment lease record");
    }
    return std::string(buffer.data(), static_cast<std::size_t>(count));
}

}  // namespace

void close_environment(const SandboxConfig& config) {
    (void)Sandbox(config);
    const Options options = box_options(config);
    const SandboxPaths paths(options);
    paths.require_marker();
    Cgroup::remove_environment(options);
    Sandbox(config).cleanup();
}

AllocatedEnvironment create_environment(const CommandLine& options) {
    if (options.channel_fd < 3 || options.environment_memory_kb == 0 || options.environment_processes == 0) {
        throw Error("create requires a channel fd and positive memory/process budgets");
    }
    fs::create_directories(lease_root);
    Fd initialization = open_lock(fs::path(lease_root) / "initialization");
    if (flock(initialization.get(), LOCK_EX) != 0) {
        fail_errno("lock environment initialization");
    }
    RuntimeConfig runtime_config = options.runtime;
    // No caller-selected UID or box number. Container-local leases coordinate
    // every environment, including clients using different base directories.
    runtime_config.instance_id = "environments";
    const PreparedRuntime runtime = prepare_runtime(runtime_config);
    for (int slot = 0; slot <= kMaxBoxId; ++slot) {
        Fd lease = open_lock(lease_path(slot));
        if (flock(lease.get(), LOCK_EX | LOCK_NB) != 0) {
            if (errno == EWOULDBLOCK || errno == EAGAIN) {
                continue;
            }
            fail_errno("acquire environment lease");
        }
        const std::string previous = read_record(lease.get());
        if (!previous.empty()) {
            close_environment({.box_id = slot, .base = previous, .cpu_set = {}, .managed_environment = true});
        }
        SandboxConfig config{.box_id = slot, .base = runtime.base,
                             .cpu_set = runtime_config.cpu_set, .managed_environment = true};
        const std::string record = config.base.string();
        if (ftruncate(lease.get(), 0) != 0 ||
            pwrite(lease.get(), record.data(), record.size(), 0) != static_cast<ssize_t>(record.size())) {
            fail_errno("record environment lease");
        }
        try {
            Cgroup::create_environment(box_options(config), options.environment_memory_kb,
                                       options.environment_processes);
            Sandbox(config).initialize();
        } catch (...) {
            close_environment(config);
            throw;
        }
        return {.config = config, .lease = std::move(lease)};
    }
    throw Error("no sandbox execution identities are available");
}

void validate_environment(const SandboxConfig& config, int lease_fd) {
    (void)Sandbox(config);
    struct stat lease{}, expected{};
    if (lease_fd < 3 || fstat(lease_fd, &lease) != 0 ||
        lstat(lease_path(config.box_id).c_str(), &expected) != 0 ||
        !S_ISREG(lease.st_mode) || lease.st_uid != 0 ||
        lease.st_dev != expected.st_dev || lease.st_ino != expected.st_ino ||
        read_record(lease_fd) != config.base.string() ||
        flock(lease_fd, LOCK_EX | LOCK_NB) != 0) {
        throw Error("invalid environment capability");
    }
}

Fd lock_execution(const SandboxConfig& config) {
    const SandboxPaths paths(box_options(config));
    paths.require_marker();
    Fd lock = open_lock(paths.control() / "execution.lock");
    if (flock(lock.get(), LOCK_EX | LOCK_NB) != 0) {
        throw Error("environment already has an active execution");
    }
    return lock;
}

void send_environment(int socket, const AllocatedEnvironment& environment) {
    std::ostringstream json;
    json << "{\"base\":" << std::quoted(environment.config.base.string())
         << ",\"slot\":" << environment.config.box_id << "}";
    const std::string body = json.str();
    iovec vector{.iov_base = const_cast<char*>(body.data()), .iov_len = body.size()};
    alignas(cmsghdr) std::array<char, CMSG_SPACE(sizeof(int))> ancillary{};
    msghdr message{};
    message.msg_iov = &vector;
    message.msg_iovlen = 1;
    message.msg_control = ancillary.data();
    message.msg_controllen = ancillary.size();
    cmsghdr* control = CMSG_FIRSTHDR(&message);
    control->cmsg_level = SOL_SOCKET;
    control->cmsg_type = SCM_RIGHTS;
    control->cmsg_len = CMSG_LEN(sizeof(int));
    const int fd = environment.lease.get();
    std::memcpy(CMSG_DATA(control), &fd, sizeof(fd));
    if (sendmsg(socket, &message, MSG_NOSIGNAL) != static_cast<ssize_t>(body.size())) {
        fail_errno("transfer environment capability");
    }
}

}  // namespace vertex::sandbox::internal
