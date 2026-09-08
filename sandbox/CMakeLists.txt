cmake_minimum_required(VERSION 3.18)
project(vertex_sandbox LANGUAGES CXX)

if(POLICY CMP0135)
    cmake_policy(SET CMP0135 NEW)
endif()

include(FetchContent)
FetchContent_Declare(
    CLI11
    URL https://github.com/CLIUtils/CLI11/archive/refs/tags/v2.6.2.tar.gz
    URL_HASH SHA256=c6ea6b2e5608b3ea8617999bd5f47420c71b2ebdb8dc4767c1034d1da5785711
)
FetchContent_MakeAvailable(CLI11)

function(vertex_sandbox_compile_options target)
    target_compile_features(${target} PRIVATE cxx_std_20)
    target_compile_options(${target} PRIVATE
        -O2
        -Wall
        -Wextra
        -Wpedantic
        -Werror
    )
    target_compile_definitions(${target} PRIVATE _FILE_OFFSET_BITS=64)
endfunction()

add_library(vertex-sandbox-lib STATIC
    src/internal/box.cpp
    src/internal/cgroup.cpp
    src/internal/error.cpp
    src/internal/environment.cpp
    src/runtime.cpp
    src/sandbox.cpp
    src/internal/security.cpp
    src/internal/supervisor.cpp
)
add_library(Vertex::Sandbox ALIAS vertex-sandbox-lib)
set_target_properties(vertex-sandbox-lib PROPERTIES
    EXPORT_NAME Sandbox
    OUTPUT_NAME vertex-sandbox
)
target_include_directories(vertex-sandbox-lib PUBLIC
    $<BUILD_INTERFACE:${CMAKE_CURRENT_SOURCE_DIR}/include>
    $<INSTALL_INTERFACE:include>
)
target_link_libraries(vertex-sandbox-lib PRIVATE seccomp)
vertex_sandbox_compile_options(vertex-sandbox-lib)
target_compile_features(vertex-sandbox-lib PUBLIC cxx_std_20)

add_executable(vertex-sandbox
    src/main.cpp
    src/internal/cli.cpp
    src/internal/options.cpp
)
target_link_libraries(vertex-sandbox PRIVATE Vertex::Sandbox CLI11::CLI11)
vertex_sandbox_compile_options(vertex-sandbox)

include(CTest)
if(BUILD_TESTING)
    add_executable(vertex-sandbox-api-test tests/api_test.cpp)
    target_link_libraries(vertex-sandbox-api-test PRIVATE Vertex::Sandbox)
    vertex_sandbox_compile_options(vertex-sandbox-api-test)
    add_test(NAME vertex-sandbox-api COMMAND vertex-sandbox-api-test)
    add_executable(vertex-sandbox-runtime-test tests/runtime_test.cpp)
    target_link_libraries(vertex-sandbox-runtime-test PRIVATE Vertex::Sandbox)
    vertex_sandbox_compile_options(vertex-sandbox-runtime-test)
    add_test(NAME vertex-sandbox-runtime COMMAND vertex-sandbox-runtime-test)
endif()

install(TARGETS vertex-sandbox RUNTIME DESTINATION bin)
install(
    TARGETS vertex-sandbox-lib
    EXPORT VertexSandboxTargets
    ARCHIVE DESTINATION lib
)
install(FILES include/vertex/sandbox.hpp DESTINATION include/vertex)
install(
    EXPORT VertexSandboxTargets
    FILE VertexSandboxTargets.cmake
    NAMESPACE Vertex::
    DESTINATION lib/cmake/VertexSandbox
)
install(
    FILES ${cli11_SOURCE_DIR}/LICENSE
    DESTINATION share/licenses/vertex-sandbox
    RENAME CLI11-LICENSE
)
