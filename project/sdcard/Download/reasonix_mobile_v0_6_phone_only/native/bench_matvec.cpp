#include "reasonix_matvec.h"
#include <chrono>
#include <cstdint>
#include <iostream>
#include <random>
#include <vector>

int main(int argc, char** argv) {
    std::size_t rows = 512, cols = 512, iters = 200;
    if (argc > 1) rows = static_cast<std::size_t>(std::stoul(argv[1]));
    if (argc > 2) cols = static_cast<std::size_t>(std::stoul(argv[2]));
    if (argc > 3) iters = static_cast<std::size_t>(std::stoul(argv[3]));
    std::mt19937 rng(3);
    std::uniform_int_distribution<int> id(-127, 127);
    std::uniform_real_distribution<float> fd(-1.0f, 1.0f);
    std::vector<std::int8_t> w(rows * cols), scratch(cols);
    std::vector<float> scales(rows, 1.0f / 127.0f), x(cols), out(rows);
    for (auto& v : w) v = static_cast<std::int8_t>(id(rng));
    for (auto& v : x) v = fd(rng);
    for (int i = 0; i < 8; ++i) reasonix::matvec_s8_per_row(w.data(), scales.data(), x.data(), out.data(), rows, cols, scratch.data());
    auto t0 = std::chrono::steady_clock::now();
    for (std::size_t i = 0; i < iters; ++i) reasonix::matvec_s8_per_row(w.data(), scales.data(), x.data(), out.data(), rows, cols, scratch.data());
    auto t1 = std::chrono::steady_clock::now();
    double sec = std::chrono::duration<double>(t1 - t0).count();
    double macs = static_cast<double>(rows) * cols * iters;
    std::cout << "rows=" << rows << " cols=" << cols << " iters=" << iters
              << " seconds=" << sec << " GMAC/s=" << (macs / sec / 1e9) << "\n";
#if defined(__aarch64__)
    std::cout << "backend=aarch64_neon";
#if defined(__ARM_FEATURE_DOTPROD)
    std::cout << "+dotprod";
#endif
    std::cout << "\n";
#else
    std::cout << "backend=scalar_host_fallback\n";
#endif
    return 0;
}
