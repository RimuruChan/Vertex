#include <vertex/internal/cgroup.hpp>

#include <vertex/internal/error.hpp>
#include <vertex/internal/supervisor.hpp>

#include <algorithm>
#include <cerrno>
#include <cstring>
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

void initialize_cpuset(const fs::path& root) {
    for (const std::string dimension : {"cpus", "mems"}) {
        const fs::path configured = root / ("cpuset." + dimension);
        std::ifstream input(configured);
        std::string value;
        if (!(input >> value)) {
            write_text(configured, read_word_file(root / ("cpuset." + dimension + ".effective")));
        }
    }
}

}  // namespace

namespace {

fs::path environment_path(const Options& options) {
    return Cgroup::root_path() / ("instance-" + options.base.filename().string()) /
           ("environment-" + std::to_string(options.box_id));
}

void remove_cgroup_tree(const fs::path& path) {
    if (!fs::exists(path)) {
        return;
    }
    const auto deadline = Clock::now() + kReapDeadline;
    while (read_keyed_uint(path / "cgroup.events", "populated") != 0) {
        if (fs::exists(path / "cgroup.kill")) {
            write_text(path / "cgroup.kill", "1");
        } else {
            for (const auto& entry : fs::directory_iterator(path)) {
                if (entry.is_directory()) {
                    remove_cgroup_tree(entry.path());
                }
            }
            std::ifstream processes(path / "cgroup.procs");
            pid_t process = 0;
            while (processes >> process) {
                if (process > 1) {
                    kill(process, SIGKILL);
                }
            }
        }
        if (Clock::now() >= deadline) {
            throw Error("environment still contains live processes: " + path.string());
        }
        std::this_thread::sleep_for(kPollInterval);
    }
    for (const auto& entry : fs::directory_iterator(path)) {
        if (entry.is_directory()) {
            remove_cgroup_tree(entry.path());
        }
    }
    if (rmdir(path.c_str()) != 0 && errno != ENOENT) {
        fail_errno("remove environment cgroup");
    }
}

}  // namespace

void Cgroup::create_environment(const Options& options, std::uint64_t memory_kb,
                                std::uint64_t processes) {
    const fs::path path = environment_path(options);
    remove_cgroup_tree(path);
    fs::create_directory(path);
    write_text(path / "memory.max", std::to_string(checked_multiply(memory_kb, 1024, "environment memory")));
    if (fs::exists(path / "memory.swap.max")) {
        write_text(path / "memory.swap.max", "0");
    }
    if (fs::exists(path / "memory.oom.group")) {
        write_text(path / "memory.oom.group", "1");
    }
    write_text(path / "pids.max", std::to_string(processes));
    std::string controllers = "+cpu +memory +pids";
    if (!options.cpu_set.empty()) {
        initialize_cpuset(path);
        write_text(path / "cpuset.cpus", options.cpu_set);
        controllers += " +cpuset";
    }
    write_text(path / "cgroup.subtree_control", controllers);
}

void Cgroup::remove_environment(const Options& options) {
    remove_cgroup_tree(environment_path(options));
}

fs::path container_cgroup_path(std::string_view membership_path) {
    const fs::path relative{membership_path};
    if (!relative.is_absolute() || relative.lexically_normal() != relative ||
        membership_path.find(" (deleted)") != std::string_view::npos ||
        std::any_of(membership_path.begin(), membership_path.end(),
                    [](unsigned char c) { return c < 32 || c == 127; })) {
        throw Error("invalid container cgroup membership");
    }
    fs::path root = "/sys/fs/cgroup";
    if (!relative.relative_path().empty()) {
        root /= relative.relative_path();
    }
    // Runtime exec joins the init process's manager leaf on re-entry.
    if (root.filename() == "vertex-manager") {
        root = root.parent_path();
    }
    return root;
}

fs::path Cgroup::container_path() {
    std::ifstream membership("/proc/self/cgroup");
    std::string line;
    while (std::getline(membership, line)) {
        if (line.starts_with("0::")) {
            return container_cgroup_path(std::string_view(line).substr(3));
        }
    }
    throw Error("cgroup v2 is required (no unified process membership)");
}

fs::path Cgroup::root_path() {
    return container_path() / "vertex-jobs";
}

void Cgroup::prepare(std::string_view instance_id, std::string_view cpu_set) {
    const fs::path container = container_path();
    const fs::path subtree = container / "cgroup.subtree_control";
    if (access(subtree.c_str(), W_OK) != 0) {
        throw Error("a writable cgroup v2 mount is required; run the worker container privileged");
    }
    std::string controllers;
    for (const std::string_view controller : {"cpu", "memory", "pids", "cpuset"}) {
        if (controller == "cpuset" && cpu_set.empty()) {
            continue;
        }
        if (!file_has_word(container / "cgroup.controllers", controller)) {
            throw Error("required cgroup controller is unavailable: " + std::string(controller));
        }
        controllers += " +" + std::string(controller);
    }

    const fs::path manager = container / "vertex-manager";
    fs::create_directory(manager);
    // Move the calling client and this helper before enabling domain controllers.
    // All threads of each process move together and inherit the container limits.
    std::ifstream processes(container / "cgroup.procs");
    if (!processes) {
        throw Error("cannot read container cgroup processes");
    }
    std::vector<pid_t> residents;
    pid_t pid = 0;
    while (processes >> pid) {
        if (pid <= 0) {
            throw Error("container cgroup contains processes outside its PID namespace");
        }
        residents.push_back(pid);
    }
    for (const pid_t resident : residents) {
        try {
            write_text(manager / "cgroup.procs", std::to_string(resident));
        } catch (const Error&) {
            if (fs::exists(fs::path("/proc") / std::to_string(resident))) {
                throw;
            }
        }
    }
    // nsdelegate protects the namespace root's cpuset limits. Leave them alone;
    // child groups inherit its effective CPU set and memory nodes.
    write_text(subtree, controllers);
    const fs::path jobs = container / "vertex-jobs";
    fs::create_directory(jobs);
    if (!cpu_set.empty()) {
        initialize_cpuset(jobs);
    }
    write_text(jobs / "cgroup.subtree_control", controllers);
    const fs::path instance = jobs / ("instance-" + std::string(instance_id));
    fs::create_directory(instance);
    if (!cpu_set.empty()) {
        initialize_cpuset(instance);
    }
    write_text(instance / "cgroup.subtree_control", controllers);
    for (const std::string_view controller : {"cpu", "memory", "pids", "cpuset"}) {
        if (controller == "cpuset" && cpu_set.empty()) {
            continue;
        }
        if (!file_has_word(instance / "cgroup.subtree_control", controller)) {
            throw Error("sandbox controller was not enabled: " + std::string(controller));
        }
    }
}

Cgroup::Cgroup(const Options& options)
    : options_(options),
      // prepare_runtime owns the instance directory and its identity. Derive
      // the matching cgroup here, without making clients pass kernel paths.
      root_(options.managed_environment ? environment_path(options) :
            root_path() / ("instance-" + options.base.filename().string())),
      path_(root_ / (options.managed_environment ? "run" : "box-" + std::to_string(options.box_id))) {}

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
