#pragma once

#include <vertex/sandbox.hpp>

#include <string>

namespace vertex::sandbox::internal {

struct Options {
    int box_id = -1;
    std::filesystem::path base = "/var/local/lib/vertex-sandbox";
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
    std::string cpu_set;
    std::string stdin_name;
    int stdin_fd = -1;
    int stdout_fd = -1;
    std::vector<std::string> environment;
    std::vector<std::string> command;
};

struct CommandLine {
    std::string action;
    SandboxConfig sandbox;
    RunOptions run;
};

CommandLine parse_options(int argc, char** argv);

}  // namespace vertex::sandbox::internal
