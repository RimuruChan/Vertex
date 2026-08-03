#pragma once

#include <chrono>
#include <filesystem>
#include <string_view>

namespace vertex::sandbox::internal {

namespace fs = std::filesystem;
using Clock = std::chrono::steady_clock;

inline constexpr int kFirstRunUid = 60000;
inline constexpr int kMaxBoxId = 4095;
inline constexpr std::string_view kBaseMarker = ".vertex-sandbox-root";
inline constexpr std::chrono::milliseconds kPollInterval{5};
inline constexpr std::chrono::milliseconds kReapDeadline{500};

}  // namespace vertex::sandbox::internal
