// Vertex exact-byte output validator. Kattis protocol: input, answer, feedback;
// team output on stdin. No token, whitespace, case or newline normalization.
#include <cstdio>
#include <fstream>
#include <iostream>
int main(int argc, char **argv) {
    if (argc < 4) return 44;
    std::ifstream answer(argv[2], std::ios::binary);
    if (!answer) return 44;
    for (;;) {
        int a = std::cin.get(), b = answer.get();
        if (std::cin.bad() || answer.bad()) return 44;
        if (a != b) return 43;
        if (a == EOF) return 42;
    }
}
