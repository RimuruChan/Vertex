cmake_minimum_required(VERSION 3.18)
project(vertex_sandbox LANGUAGES CXX)

add_executable(vertex-sandbox main.cpp)
target_compile_features(vertex-sandbox PRIVATE cxx_std_20)
target_compile_options(vertex-sandbox PRIVATE
    -O2
    -Wall
    -Wextra
    -Wpedantic
    -Werror
)
target_compile_definitions(vertex-sandbox PRIVATE _FILE_OFFSET_BITS=64)
target_link_libraries(vertex-sandbox PRIVATE seccomp)

install(TARGETS vertex-sandbox RUNTIME DESTINATION bin)
