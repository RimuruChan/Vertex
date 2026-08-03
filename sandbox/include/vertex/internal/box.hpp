#pragma once

#include <vertex/internal/constants.hpp>
#include <vertex/internal/fd.hpp>
#include <vertex/internal/options.hpp>

namespace vertex::sandbox::internal {

class SandboxPaths {
public:
    explicit SandboxPaths(const Options& options);

    [[nodiscard]] const fs::path& base() const { return base_; }
    [[nodiscard]] const fs::path& root() const { return root_; }
    [[nodiscard]] const fs::path& workspace() const { return workspace_; }
    [[nodiscard]] const fs::path& control() const { return control_; }

    void ensure_base() const;
    void require_marker() const;

private:
    fs::path base_;
    fs::path root_;
    fs::path workspace_;
    fs::path control_;
};

void validate_relative_name(const std::string& name);
Fd open_input(const Options& options, const SandboxPaths& paths);
Fd open_control_output(const Options& options, const SandboxPaths& paths,
                       std::string_view name);
void initialize_box(const Options& options);
void cleanup_box(const Options& options);

}  // namespace vertex::sandbox::internal
