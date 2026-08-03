// Vertex sandbox runner.
//
// The implementation lives behind a narrow CLI facade. See the module list in
// README.md for the security and supervision boundaries.

#include <vertex/internal/cli.hpp>

int main(int argc, char** argv) {
    return vertex::sandbox::internal::run_cli(argc, argv);
}
