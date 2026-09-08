#include <vertex/sandbox.hpp>

#include <vertex/internal/box.hpp>
#include <vertex/internal/cgroup.hpp>
#include <vertex/internal/error.hpp>
#include <vertex/internal/security.hpp>

#include <algorithm>
#include <array>
#include <unistd.h>

namespace vertex::sandbox {
namespace {

bool alphanumeric(unsigned char c) {
    return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
           (c >= '0' && c <= '9');
}

std::string instance_id(std::string configured) {
    if (configured.empty()) {
        std::array<char, 256> hostname{};
        if (gethostname(hostname.data(), hostname.size() - 1) != 0) {
            internal::fail_errno("read hostname");
        }
        configured = hostname.data();
    }
    if (configured.empty() || configured.size() > 64 || !alphanumeric(configured.front()) ||
        !std::all_of(configured.begin(), configured.end(), [](unsigned char c) {
            return alphanumeric(c) || c == '_' || c == '.' || c == '-';
        })) {
        throw internal::Error("instance ID must be 1-64 safe characters starting with a letter or digit");
    }
    return configured;
}

}  // namespace

PreparedRuntime prepare_runtime(const RuntimeConfig& config) {
    const std::string id = instance_id(config.instance_id);
    const std::string base = config.base.string();
    if (std::any_of(base.begin(), base.end(), [](unsigned char c) { return c < 32 || c == 127; })) {
        throw internal::Error("sandbox base must not contain control characters");
    }
    // Reuse the same base and CPU-set validation as individual box operations.
    (void)Sandbox({.box_id = 0, .base = config.base, .cpu_set = config.cpu_set});
    internal::Options options;
    options.base = config.base;
    (void)internal::SandboxPaths(options);
    if (internal::landlock_abi() < 1) {
        internal::fail_errno("Landlock is required");
    }

    internal::Cgroup::prepare(id, config.cpu_set);
    PreparedRuntime runtime{
        .base = config.base / "instances" / id,
        .container_cgroup = internal::Cgroup::container_path(),
        .cgroup_root = internal::Cgroup::root_path() / ("instance-" + id),
        .instance_id = id,
    };
    options.base = runtime.base;
    internal::SandboxPaths(options).ensure_base();
    return runtime;
}

}  // namespace vertex::sandbox
