#include "reasonix_matvec.h"
#include <algorithm>
#include <cmath>
#if defined(__aarch64__)
#include <arm_neon.h>
#endif
namespace reasonix {
float quantize_s8(const float* x,std::int8_t* q,std::size_t n){
    float m=0; for(std::size_t i=0;i<n;++i)m=std::max(m,std::fabs(x[i]));
    const float s=m>1e-12f?m/127.0f:1.0f, inv=1.0f/s;
    for(std::size_t i=0;i<n;++i){ long v=std::lround(x[i]*inv); v=std::max<long>(-127,std::min<long>(127,v)); q[i]=static_cast<std::int8_t>(v);} return s;
}
std::int32_t dot_s8(const std::int8_t*a,const std::int8_t*b,std::size_t n){
    std::int32_t sum=0; std::size_t i=0;
#if defined(__aarch64__)
    int32x4_t acc=vdupq_n_s32(0);
#if defined(__ARM_FEATURE_DOTPROD)
    for(;i+16<=n;i+=16)acc=vdotq_s32(acc,vld1q_s8(a+i),vld1q_s8(b+i));
#else
    for(;i+16<=n;i+=16){auto va=vld1q_s8(a+i),vb=vld1q_s8(b+i);auto p0=vmull_s8(vget_low_s8(va),vget_low_s8(vb));auto p1=vmull_s8(vget_high_s8(va),vget_high_s8(vb));acc=vaddq_s32(acc,vpaddlq_s16(p0));acc=vaddq_s32(acc,vpaddlq_s16(p1));}
#endif
    sum+=vaddvq_s32(acc);
#endif
    for(;i<n;++i) sum+=static_cast<std::int32_t>(a[i])*static_cast<std::int32_t>(b[i]);
    return sum;
}
void matvec_s8_per_row(const std::int8_t*w,const float*rs,const float*x,float*out,std::size_t rows,std::size_t cols,std::int8_t*scratch){
    const float xs=quantize_s8(x,scratch,cols); for(std::size_t r=0;r<rows;++r)out[r]=static_cast<float>(dot_s8(w+r*cols,scratch,cols))*rs[r]*xs;
}
static inline int s4(std::uint8_t b,bool hi){int v=hi?((b>>4)&15):(b&15);return v>=8?v-16:v;}
void matvec_s4_group_per_row(const std::uint8_t*packed,const float*scales,const float*x,float*out,std::size_t rows,std::size_t cols,std::size_t group){
    const std::size_t row_bytes=(cols+1)/2, ng=(cols+group-1)/group;
    for(std::size_t r=0;r<rows;++r){float total=0;const auto*pr=packed+r*row_bytes;for(std::size_t g=0;g<ng;++g){const std::size_t s=g*group,e=std::min(cols,s+group);float acc=0;for(std::size_t c=s;c<e;++c){const auto b=pr[c/2];acc+=static_cast<float>(s4(b,(c&1)!=0))*x[c];}total+=acc*scales[r*ng+g];}out[r]=total;}
}
} // namespace
