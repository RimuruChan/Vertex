#include <vertex/internal/box.hpp>

#include <vertex/internal/cgroup.hpp>
#include <vertex/internal/error.hpp>
#include <vertex/internal/security.hpp>

#include <cerrno>
#include <fcntl.h>
#include <system_error>

#include <sys/stat.h>

namespace vertex::sandbox::internal {
namespace {

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

}  // namespace

SandboxPaths::SandboxPaths(const Options& options)
    : base_(options.base),
      root_(base_ / std::to_string(options.box_id)),
      workspace_(root_ / "box"),
      control_(root_ / "control") {
    validate_base(base_);
}

void SandboxPaths::require_marker() const {
    const fs::path marker = base_ / kBaseMarker;
    struct stat status {};
    if (lstat(marker.c_str(), &status) != 0 || !S_ISREG(status.st_mode) ||
        status.st_uid != 0 || (status.st_mode & 0022) != 0) {
        throw Error("sandbox base is missing its trusted marker: " + base_.string());
    }
}

void SandboxPaths::ensure_base() const {
    fs::create_directories(base_);
    const fs::path marker = base_ / kBaseMarker;
    if (!fs::exists(marker)) {
        if (fs::directory_iterator(base_) != fs::directory_iterator{}) {
            throw Error("refusing to initialize a non-empty sandbox base: " +
                        base_.string());
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
    require_marker();
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

Fd open_input(const Options& options, const SandboxPaths& paths) {
    if (options.stdin_fd >= 0) {
        const int flags = fcntl(options.stdin_fd, F_GETFL);
        struct stat status {};
        if (flags < 0 || fstat(options.stdin_fd, &status) != 0 ||
            (flags & O_ACCMODE) == O_WRONLY || S_ISDIR(status.st_mode)) {
            throw Error("stdin fd must be a readable inherited descriptor");
        }
        return Fd(options.stdin_fd);
    }
    if (options.stdin_name.empty()) {
        const int fd = open("/dev/null", O_RDONLY | O_CLOEXEC);
        if (fd < 0) {
            fail_errno("open /dev/null");
        }
        return Fd(fd);
    }
    validate_relative_name(options.stdin_name);
    const fs::path path = paths.workspace() / options.stdin_name;
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

Fd open_control_output(const Options& options, const SandboxPaths& paths,
                       std::string_view name) {
    if (name == "stdout" && options.stdout_fd >= 0) {
        const int flags = fcntl(options.stdout_fd, F_GETFL);
        struct stat status {};
        if (flags < 0 || fstat(options.stdout_fd, &status) != 0 ||
            (flags & O_ACCMODE) == O_RDONLY || S_ISDIR(status.st_mode)) {
            throw Error("stdout fd must be a writable inherited descriptor");
        }
        return Fd(options.stdout_fd);
    }
    const fs::path path = paths.control() / name;
    const int fd = open(path.c_str(), O_WRONLY | O_CREAT | O_TRUNC | O_CLOEXEC |
                                      O_NOFOLLOW,
                        0644);
    if (fd < 0) {
        fail_errno("open output " + path.string());
    }
    return Fd(fd);
}

void initialize_box(const Options& options) {
    if (landlock_abi() < 1) {
        fail_errno("Landlock is required");
    }
    const SandboxPaths paths(options);
    paths.ensure_base();
    Cgroup cgroup(options);
    cgroup.validate_cpu_set();
    // Init is safe as a standalone recovery operation too: never replace a
    // workspace while descendants from an earlier runner may still use it.
    cgroup.remove();
    std::error_code error;
    fs::remove_all(paths.root(), error);
    if (error) {
        throw Error("remove stale box " + paths.root().string() + ": " +
                    error.message());
    }
    fs::create_directories(paths.workspace());
    fs::create_directory(paths.control());
    if (chmod(paths.root().c_str(), 0755) != 0 ||
        chmod(paths.workspace().c_str(), 0700) != 0 ||
        chmod(paths.control().c_str(), 0700) != 0) {
        fail_errno("set box permissions");
    }
    const uid_t uid = static_cast<uid_t>(kFirstRunUid + options.box_id);
    const gid_t gid = static_cast<gid_t>(kFirstRunUid + options.box_id);
    if (chown(paths.workspace().c_str(), uid, gid) != 0) {
        fail_errno("set box ownership");
    }
}

void cleanup_box(const Options& options) {
    const SandboxPaths paths(options);
    paths.require_marker();
    Cgroup(options).remove();
    std::error_code error;
    fs::remove_all(paths.root(), error);
    if (error) {
        throw Error("remove box " + paths.root().string() + ": " + error.message());
    }
}

}  // namespace vertex::sandbox::internal
