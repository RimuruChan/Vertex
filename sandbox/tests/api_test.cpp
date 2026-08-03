#include <vertex/sandbox.hpp>

#include <filesystem>
#include <stdexcept>

int main() {
    vertex::sandbox::CancellationToken cancellation;
    if (cancellation.requested()) {
        return 1;
    }
    cancellation.request();
    if (!cancellation.requested()) {
        return 1;
    }

    vertex::sandbox::SandboxConfig config;
    config.box_id = 7;
    const vertex::sandbox::Sandbox sandbox(config);
    if (sandbox.workspace_path() !=
        std::filesystem::path{"/var/local/lib/vertex-sandbox/7/box"}) {
        return 1;
    }

    try {
        const vertex::sandbox::Sandbox invalid({});
        (void)invalid;
        return 1;
    } catch (const std::runtime_error&) {
    }

    try {
        (void)sandbox.run({});
        return 1;
    } catch (const std::runtime_error&) {
    }
    return 0;
}
