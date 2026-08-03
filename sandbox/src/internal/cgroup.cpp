#include <vertex/internal/cgroup.hpp>

#include <vertex/internal/error.hpp>
#include <vertex/internal/supervisor.hpp>

#include <algorithm>
#include <cerrno>
#include <cstring>
#include <cstdlib>
#include <fstream>
#include <optional>
#include <thread>

namespace vertex::sandbox::internal {
namespace {

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

std::string read_word_file(const fs::path& path) {
    std::ifstream input(path);
    std::string value;
    if (!(input >> value)) {
        throw Error("read " + path.string());
    }
    return value;
}

bool file_has_word(const fs::path& path, std::string_view wanted) {
    std::ifstream input(path);
    std::string value;
    while (input >> value) {
        if (value == wanted) {
            return true;
        }
    }
    return false;
}

}  // namespace

fs::path Cgroup::root_path() {
    if (const char* configured = std::getenv("VERTEX_CGROUP_ROOT");
        configured && *configured) {
        return configured;
    }
    return "/vertex-cgroup";
}

Cgroup::Cgroup(const Options& options)
    : options_(options),
      root_(root_path()),
      path_(root_ / ("box-" + std::to_string(options.box_id))) {}

Cgroup::~Cgroup() noexcept {
    if (!owned_) {
        return;
    }
    try {
        remove();
    } catch (...) {
    }
}

void Cgroup::validate_cpu_set() const {
    if (options_.cpu_set.empty()) {
        return;
    }
    if (!file_has_word(root_ / "cgroup.controllers", "cpuset") ||
        !file_has_word(root_ / "cgroup.subtree_control", "cpuset")) {
        throw Error("configured CPU set requires a delegated cpuset controller");
    }
    (void)read_word_file(root_ / "cpuset.mems.effective");
}

void Cgroup::create() {
    if (!fs::exists(root_ / "cgroup.controllers")) {
        throw Error("writable delegated cgroup v2 root is required: " + root_.string());
    }
    remove();
    if (!fs::create_directory(path_)) {
        throw Error("create cgroup: " + path_.string());
    }
    owned_ = true;
    try {
        if (!options_.cpu_set.empty()) {
            validate_cpu_set();
            if (!fs::exists(path_ / "cpuset.mems") ||
                !fs::exists(path_ / "cpuset.cpus")) {
                throw Error("cpuset controller is not active for child cgroups");
            }
            write_text(path_ / "cpuset.mems",
                       read_word_file(root_ / "cpuset.mems.effective"));
            write_text(path_ / "cpuset.cpus", options_.cpu_set);
        }
        write_text(path_ / "memory.max",
                   std::to_string(checked_multiply(options_.memory_kb, 1024,
                                                   "memory limit")));
        if (fs::exists(path_ / "memory.swap.max")) {
            write_text(path_ / "memory.swap.max", "0");
        }
        if (fs::exists(path_ / "memory.oom.group")) {
            write_text(path_ / "memory.oom.group", "1");
        }
        write_text(path_ / "pids.max", std::to_string(options_.processes));
    } catch (...) {
        try {
            remove();
        } catch (...) {
        }
        throw;
    }
}

void Cgroup::add_process(pid_t process) const {
    write_text(path_ / "cgroup.procs", std::to_string(process));
}

void Cgroup::kill_all() const {
    std::error_code ignored;
    if (fs::exists(path_ / "cgroup.kill", ignored)) {
        try {
            write_text(path_ / "cgroup.kill", "1");
            return;
        } catch (const Error&) {
            // Fall back to enumerating cgroup.procs below.
        }
    }
    std::ifstream processes(path_ / "cgroup.procs");
    pid_t process = 0;
    while (processes >> process) {
        if (process > 1) {
            kill(process, SIGKILL);
        }
    }
}

bool Cgroup::populated() const {
    return read_keyed_uint(path_ / "cgroup.events", "populated") != 0;
}

void Cgroup::remove() {
    std::error_code ignored;
    if (!fs::exists(path_, ignored)) {
        owned_ = false;
        return;
    }
    const auto deadline = Clock::now() + kReapDeadline;
    while (populated() && Clock::now() < deadline) {
        // cgroup.kill is unavailable on some otherwise supported cgroup v2
        // kernels. Re-enumeration closes the fork race in that fallback path.
        kill_all();
        std::this_thread::sleep_for(kPollInterval);
    }
    kill_all();
    if (rmdir(path_.c_str()) != 0 && errno != ENOENT) {
        throw Error("remove cgroup " + path_.string() + ": " +
                    std::strerror(errno));
    }
    owned_ = false;
}

void Cgroup::update_stats(RunStats& stats) const {
    stats.cpu_usec = std::max(stats.cpu_usec,
                              read_keyed_uint(path_ / "cpu.stat", "usage_usec"));
    if (const auto peak = read_uint_file(path_ / "memory.peak")) {
        stats.memory_peak_bytes = std::max(stats.memory_peak_bytes, *peak);
    } else if (const auto current = read_uint_file(path_ / "memory.current")) {
        stats.memory_peak_bytes = std::max(stats.memory_peak_bytes, *current);
    }
    stats.oom_killed = read_keyed_uint(path_ / "memory.events", "oom_kill") > 0;
}

}  // namespace vertex::sandbox::internal
