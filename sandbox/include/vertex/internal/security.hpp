#pragma once

#include <vertex/internal/options.hpp>

#include <sys/types.h>

namespace vertex::sandbox::internal {

int landlock_abi();
[[noreturn]] void execute_child(const Options& options, pid_t runner_pid,
                                int sync_fd, int error_fd, int stdin_fd,
                                int stdout_fd, int stderr_fd);

}  // namespace vertex::sandbox::internal
