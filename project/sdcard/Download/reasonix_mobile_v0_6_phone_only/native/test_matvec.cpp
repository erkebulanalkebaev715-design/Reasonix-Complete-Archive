#include "reasonix_matvec.h"
#include <cmath>
#include <cstdint>
#include <iostream>
#include <vector>
int main(){
    // INT8 primitive sanity.
    const std::size_t rows=2,cols=8;float x[cols]={.1f,-.2f,.3f,-.4f,.5f,-.6f,.7f,-.8f};
    std::int8_t w[rows*cols]={1,2,3,4,5,6,7,8,-2,-1,0,1,2,3,4,5};float s[rows]={.01f,.02f},o[rows];std::int8_t scratch[cols];reasonix::matvec_s8_per_row(w,s,x,o,rows,cols,scratch);
    if(!std::isfinite(o[0])||!std::isfinite(o[1]))return 1;
    // INT4 exact hand-check. row values [1,-2,3,-4], scale .5 => [.5,-1,1.5,-2]
    const std::uint8_t p[2]={0xE1,0xC3}; const float gs[1]={.5f}; const float x4[4]={1,2,3,4}; float y4[1];
    reasonix::matvec_s4_group_per_row(p,gs,x4,y4,1,4,4);
    const float ref=.5f*1.f + (-1.f)*2.f + 1.5f*3.f + (-2.f)*4.f;
    const float err=std::fabs(y4[0]-ref); std::cout<<"int4_abs_error="<<err<<"\n";
    if(err>1e-6f)return 2;
    std::cout<<"MATVEC_V06_PASS\n";return 0;
}
