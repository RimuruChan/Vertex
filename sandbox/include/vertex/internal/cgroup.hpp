#pragma once

#include <vertex/internal/constants.hpp>
#include <vertex/internal/options.hpp>

#include <string_view>
#include <sys/types.h>

namespace vertex::sandbox::internal {

struct RunStats;

[[nodiscard]] fs::path container_cgroup_path(std::string_view membership_path);

class Cgroup {
public:
    explicit Cgroup(const Options& options);
    ~Cgroup() noexcept;
    Cgroup(const Cgroup&) = delete;
    Cgroup& operator=(const Cgroup&) = delete;

    [[nodiscard]] static fs::path root_path();
    [[nodiscard]] static fs::path container_path();
    static void prepare(std::string_view instance_id, std::string_view cpu_set);
    static void create_environment(const Options& options, std::uint64_t memory_kb, std::uint64_t processes);
    static void remove_environment(const Options& options);
    [[nodiscard]] const fs::path& path() const { return path_; }

    void validate_cpu_set() const;
    void create();
    void add_process(pid_t process) const;
    void kill_all() const;
    void remove();
    void update_stats(RunStats& stats) const;

private:
    [[nodiscard]] bool populated() const;

    const Options& options_;
    fs::path root_;
    fs::path path_;
    bool owned_ = false;
};

}  // namespace vertex::sandbox::internal
