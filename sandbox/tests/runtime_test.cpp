#include <vertex/internal/cgroup.hpp>
#include <vertex/sandbox.hpp>

#include <array>
#include <stdexcept>
#include <string>

int main() {
    using vertex::sandbox::internal::container_cgroup_path;
    if (container_cgroup_path("/") != "/sys/fs/cgroup" ||
        container_cgroup_path("/vertex-manager") != "/sys/fs/cgroup" ||
        container_cgroup_path("/pod/container") != "/sys/fs/cgroup/pod/container" ||
        container_cgroup_path("/pod/container/vertex-manager") != "/sys/fs/cgroup/pod/container") {
        return 1;
    }
    for (const std::string_view path : {"", "relative", "/../host", "/pod/../host", "/gone (deleted)", "/pod\nbad"}) {
        try {
            (void)container_cgroup_path(path);
            return 1;
        } catch (const std::runtime_error&) {
        }
    }
    // Invalid configuration must fail before kernel probing or filesystem writes.
    for (const std::string& id : std::array<std::string, 4>{"../escape", ".hidden", "bad/id", std::string(65, 'a')}) {
        try {
            (void)vertex::sandbox::prepare_runtime({.instance_id = id, .cpu_set = {}});
            return 1;
        } catch (const std::runtime_error& error) {
            if (std::string(error.what()).find("instance ID") == std::string::npos) {
                return 1;
            }
        }
    }
    for (const std::string_view path : {"/", "relative", "/tmp/../var/data", "/tmp/bad\npath"}) {
        try {
            (void)vertex::sandbox::prepare_runtime({.base = path, .instance_id = "test", .cpu_set = {}});
            return 1;
        } catch (const std::runtime_error& error) {
            if (std::string(error.what()).find("sandbox base") == std::string::npos) {
                return 1;
            }
        }
    }
    return 0;
}
