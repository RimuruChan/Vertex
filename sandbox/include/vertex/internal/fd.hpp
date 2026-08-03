#pragma once

#include <unistd.h>

namespace vertex::sandbox::internal {

class Fd {
public:
    explicit Fd(int value = -1) : value_(value) {}
    ~Fd() {
        if (value_ >= 0) {
            close(value_);
        }
    }
    Fd(const Fd&) = delete;
    Fd& operator=(const Fd&) = delete;
    Fd(Fd&& other) noexcept : value_(other.release()) {}
    Fd& operator=(Fd&& other) noexcept {
        if (this != &other) {
            if (value_ >= 0) {
                close(value_);
            }
            value_ = other.release();
        }
        return *this;
    }

    [[nodiscard]] int get() const { return value_; }
    [[nodiscard]] int release() {
        const int value = value_;
        value_ = -1;
        return value;
    }

private:
    int value_;
};

}  // namespace vertex::sandbox::internal
