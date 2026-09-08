#include <vertex/internal/cli.hpp>

#include <vertex/internal/error.hpp>
#include <vertex/internal/environment.hpp>
#include <vertex/internal/options.hpp>
#include <vertex/sandbox.hpp>

#include <csignal>
#include <iomanip>
#include <iostream>

namespace vertex::sandbox::internal {
namespace {

CancellationToken cancellation;

void signal_handler(int signal_number) {
    (void)signal_number;
    cancellation.request();
}

void install_signal_handlers() {
    struct sigaction action {};
    action.sa_handler = signal_handler;
    sigemptyset(&action.sa_mask);
    for (const int signal_number : {SIGINT, SIGTERM, SIGHUP}) {
        if (sigaction(signal_number, &action, nullptr) != 0) {
            fail_errno("install signal handler");
        }
    }
}

}  // namespace

int run_cli(int argc, char** argv) noexcept {
    try {
        const CommandLine options = parse_options(argc, argv);
        install_signal_handlers();

        if (options.action == "create") {
            AllocatedEnvironment environment = create_environment(options);
            try {
                send_environment(options.channel_fd, environment);
            } catch (...) {
                close_environment(environment.config);
                throw;
            }
            return 0;
        }
        if (options.action == "close" || options.action == "start") {
            validate_environment(options.sandbox, options.lease_fd);
            if (options.action == "close") {
                close_environment(options.sandbox);
            } else {
                const Fd active = lock_execution(options.sandbox);
                (void)Sandbox(options.sandbox).run(options.run, &cancellation);
            }
            return 0;
        }

        if (options.action == "prepare") {
            const PreparedRuntime runtime = prepare_runtime(options.runtime);
            std::cout << "{\"base\":" << std::quoted(runtime.base.string())
                      << ",\"containerCgroup\":" << std::quoted(runtime.container_cgroup.string())
                      << ",\"cgroupRoot\":" << std::quoted(runtime.cgroup_root.string())
                      << ",\"instanceId\":" << std::quoted(runtime.instance_id) << "}\n";
            return 0;
        }

        if (options.action == "probe") {
            const Capabilities capabilities = Sandbox::probe();
            std::cout << "landlock-abi=" << capabilities.landlock_abi
                      << " cgroup-v2=" << capabilities.cgroup_root << '\n';
            return 0;
        }

        const Sandbox sandbox(options.sandbox);
        if (options.action == "init") {
            sandbox.initialize();
        } else if (options.action == "cleanup") {
            sandbox.cleanup();
        } else {
            (void)sandbox.run(options.run, &cancellation);
        }
        return 0;
    } catch (const std::exception& error) {
        std::cerr << "vertex-sandbox: " << error.what() << '\n';
        return 2;
    }
}

}  // namespace vertex::sandbox::internal
