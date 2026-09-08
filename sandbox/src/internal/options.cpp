#include <vertex/internal/options.hpp>

#include <vertex/internal/error.hpp>

#include <CLI/CLI.hpp>

namespace vertex::sandbox::internal {
namespace {

void add_box_options(CLI::App& command, SandboxConfig& options) {
    command.add_option("--box-id", options.box_id, "Sandbox box identifier");
    command.add_option("--base", options.base, "Sandbox state directory");
    command.add_option("--cpu-set", options.cpu_set, "Optional cgroup CPU set");
}

void add_run_options(CLI::App& command, CommandLine& options) {
    add_box_options(command, options.sandbox);
    RunOptions& run = options.run;
    command.add_option("--time-ms", run.time_ms, "Soft CPU limit in milliseconds");
    command.add_option("--time-hard-ms", run.time_hard_ms,
                       "Hard CPU limit in milliseconds");
    command.add_option("--wall-ms", run.wall_ms, "Soft wall limit in milliseconds");
    command.add_option("--wall-hard-ms", run.wall_hard_ms,
                       "Hard wall limit in milliseconds");
    command.add_option("--memory-kb", run.memory_kb, "Memory limit in KiB");
    command.add_option("--processes", run.processes, "Process limit");
    command.add_option("--output-bytes", run.output_bytes,
                       "Per-stream output limit in bytes");
    command.add_option("--workspace-bytes", run.workspace_bytes,
                       "Aggregate workspace limit in bytes");
    command.add_option("--workspace-inodes", run.workspace_inodes,
                       "Aggregate workspace inode limit");
    command.add_option("--stack-kb", run.stack_kb, "Stack limit in KiB");
    command.add_option("--stdin", run.stdin_name, "Workspace input file");
    command.add_option("--stdin-fd", run.stdin_fd, "Inherited input descriptor");
    command.add_option("--stdout-fd", run.stdout_fd, "Inherited output descriptor");
    command.add_option("--env", run.environment, "Child environment entry");
}

}  // namespace

CommandLine parse_options(int argc, char** argv) {
    if (argc < 2) {
        throw Error("expected one of: prepare, probe, init, cleanup, run");
    }

    CommandLine options;
    int parser_argc = argc;
    if (std::string_view(argv[1]) == "run" || std::string_view(argv[1]) == "start") {
        for (int index = 2; index < argc; ++index) {
            if (std::string_view(argv[index]) == "--") {
                parser_argc = index;
                options.run.command.assign(argv + index + 1, argv + argc);
                break;
            }
        }
        if (parser_argc == argc) {
            throw Error("run requires a command after --");
        }
    }

    CLI::App app{"Vertex sandbox runner", "vertex-sandbox"};
    app.set_help_flag();
    app.require_subcommand(1, 1);

    CLI::App* probe_command = app.add_subcommand("probe", "Probe kernel features");
    CLI::App* prepare_command = app.add_subcommand("prepare", "Prepare container execution resources");
    prepare_command->add_option("--base", options.runtime.base, "Parent directory for sandbox instances");
    prepare_command->add_option("--instance-id", options.runtime.instance_id, "Instance identity (default: hostname)");
    prepare_command->add_option("--cpu-set", options.runtime.cpu_set, "Optional cgroup CPU set");
    CLI::App* init_command = app.add_subcommand("init", "Initialize a sandbox box");
    CLI::App* cleanup_command = app.add_subcommand("cleanup", "Remove a sandbox box");
    CLI::App* run_command = app.add_subcommand("run", "Run a sandboxed command");
    CLI::App* create_command = app.add_subcommand("create", "Create an isolated environment");
    create_command->add_option("--base", options.runtime.base);
    create_command->add_option("--cpu-set", options.runtime.cpu_set);
    create_command->add_option("--channel-fd", options.channel_fd)->required();
    create_command->add_option("--memory-kb", options.environment_memory_kb)->required();
    create_command->add_option("--processes", options.environment_processes)->required();
    CLI::App* start_command = app.add_subcommand("start", "Start a process in an environment");
    add_run_options(*start_command, options);
    start_command->add_option("--lease-fd", options.lease_fd)->required();
    CLI::App* close_command = app.add_subcommand("close", "Destroy an environment");
    add_box_options(*close_command, options.sandbox);
    close_command->add_option("--lease-fd", options.lease_fd)->required();
    add_box_options(*init_command, options.sandbox);
    add_box_options(*cleanup_command, options.sandbox);
    add_run_options(*run_command, options);

    try {
        app.parse(parser_argc, argv);
    } catch (const CLI::ParseError& error) {
        throw Error(error.what());
    }

    if (*create_command) {
        options.action = "create";
    } else if (*start_command || *close_command) {
        options.action = *start_command ? "start" : "close";
        options.sandbox.managed_environment = true;
    } else if (*prepare_command) {
        options.action = "prepare";
    } else if (*probe_command) {
        options.action = "probe";
    } else if (*init_command) {
        options.action = "init";
    } else if (*cleanup_command) {
        options.action = "cleanup";
    } else {
        options.action = "run";
    }
    return options;
}

}  // namespace vertex::sandbox::internal
