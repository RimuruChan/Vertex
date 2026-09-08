#pragma once

#include <csignal>
#include <cstdint>
#include <filesystem>
#include <string>
#include <vector>

namespace vertex::sandbox {

struct RuntimeConfig {
    std::filesystem::path base = "/vertex/sandbox";
    std::string instance_id;
    std::string cpu_set;
};

struct PreparedRuntime {
    std::filesystem::path base;
    std::filesystem::path container_cgroup;
    std::filesystem::path cgroup_root;
    std::string instance_id;
};

// Prepare the container's execution resources without resetting existing boxes.
// This is an internal bootstrap primitive; application clients create environments.
[[nodiscard]] PreparedRuntime prepare_runtime(const RuntimeConfig& config);

struct SandboxConfig {
    int box_id = -1;
    std::filesystem::path base = "/vertex/sandbox";
    std::string cpu_set;
    bool managed_environment = false;
};

struct RunOptions {
    std::uint64_t time_ms = 0;
    std::uint64_t time_hard_ms = 0;
    std::uint64_t wall_ms = 0;
    std::uint64_t wall_hard_ms = 0;
    std::uint64_t memory_kb = 0;
    std::uint64_t processes = 1;
    std::uint64_t output_bytes = 0;
    std::uint64_t workspace_bytes = 64ULL * 1024 * 1024;
    std::uint64_t workspace_inodes = 4096;
    std::uint64_t stack_kb = 0;
    std::string stdin_name;
    int stdin_fd = -1;
    int stdout_fd = -1;
    std::vector<std::string> environment;
    std::vector<std::string> command;
};

struct Capabilities {
    int landlock_abi = 0;
    std::filesystem::path cgroup_root;
};

struct RunArtifacts {
    std::filesystem::path meta;
    std::filesystem::path stdout_file;
    std::filesystem::path stderr_file;
};

class CancellationToken {
public:
    CancellationToken() = default;
    CancellationToken(const CancellationToken&) = delete;
    CancellationToken& operator=(const CancellationToken&) = delete;

    void request() noexcept;
    [[nodiscard]] bool requested() const noexcept;

private:
    volatile std::sig_atomic_t requested_ = 0;
};

class Sandbox {
public:
    explicit Sandbox(SandboxConfig config);

    [[nodiscard]] static Capabilities probe();
    [[nodiscard]] const SandboxConfig& config() const noexcept { return config_; }
    [[nodiscard]] std::filesystem::path workspace_path() const;

    void initialize() const;
    void cleanup() const;
    [[nodiscard]] RunArtifacts run(
        RunOptions options,
        const CancellationToken* cancellation = nullptr) const;

private:
    SandboxConfig config_;
};

}  // namespace vertex::sandbox
