#include <vertex/internal/error.hpp>

#include <cerrno>
#include <cstring>
#include <limits>

namespace vertex::sandbox::internal {

[[noreturn]] void fail_errno(std::string_view operation) {
    throw Error(std::string(operation) + ": " + std::strerror(errno));
}

std::uint64_t checked_multiply(std::uint64_t value, std::uint64_t factor,
                               std::string_view name) {
    if (value > std::numeric_limits<std::uint64_t>::max() / factor) {
        throw Error(std::string(name) + " is too large");
    }
    return value * factor;
}

}  // namespace vertex::sandbox::internal
