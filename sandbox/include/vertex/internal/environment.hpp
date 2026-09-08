#pragma once

#include <vertex/internal/fd.hpp>
#include <vertex/internal/options.hpp>

namespace vertex::sandbox::internal {

struct AllocatedEnvironment {
    SandboxConfig config;
    Fd lease;
};

AllocatedEnvironment create_environment(const CommandLine& options);
void send_environment(int socket, const AllocatedEnvironment& environment);
void validate_environment(const SandboxConfig& config, int lease_fd);
void close_environment(const SandboxConfig& config);
Fd lock_execution(const SandboxConfig& config);

}  // namespace vertex::sandbox::internal
