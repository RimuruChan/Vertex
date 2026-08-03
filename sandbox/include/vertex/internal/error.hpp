#pragma once

#include <cstdint>
#include <stdexcept>
#include <string>
#include <string_view>

namespace vertex::sandbox::internal {

class Error : public std::runtime_error {
public:
    explicit Error(const std::string& message) : std::runtime_error(message) {}
};

[[noreturn]] void fail_errno(std::string_view operation);
std::uint64_t checked_multiply(std::uint64_t value, std::uint64_t factor,
                               std::string_view name);

}  // namespace vertex::sandbox::internal
