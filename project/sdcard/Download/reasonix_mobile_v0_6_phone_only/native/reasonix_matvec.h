#pragma once
#include <cstddef>
#include <cstdint>
namespace reasonix {
float quantize_s8(const float* x, std::int8_t* q, std::size_t n);
std::int32_t dot_s8(const std::int8_t* a, const std::int8_t* b, std::size_t n);
void matvec_s8_per_row(const std::int8_t* weights,const float* row_scales,const float* x,float* out,std::size_t rows,std::size_t cols,std::int8_t* x_scratch);
void matvec_s4_group_per_row(const std::uint8_t* packed,const float* group_scales,const float* x,float* out,std::size_t rows,std::size_t cols,std::size_t group_size);
}
