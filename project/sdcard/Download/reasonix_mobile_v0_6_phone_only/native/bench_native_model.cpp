#include "reasonix_native_model.h"
#include <chrono>
#include <cstdint>
#include <iostream>
#include <string>
#include <sys/resource.h>
#include <sys/stat.h>

static long rss_kb(){ struct rusage r{}; return getrusage(RUSAGE_SELF,&r)==0?r.ru_maxrss:-1; }
int main(int argc,char**argv){
    if(argc<2){std::cerr<<"usage: bench-native-model <model.rxm6> [iters]\n";return 2;}
    const int iters=argc>=3?std::stoi(argv[2]):500;
    try{
        struct stat st{}; stat(argv[1],&st);
        auto w=reasonix::NativeWeights::load(argv[1]);reasonix::NativeReasonix m(std::move(w));
        std::cout<<"model_bytes="<<st.st_size<<"\n"<<"mapped_bytes="<<m.mapped_bytes()<<"\n";
        for(const std::string mode:{"fast","standard","deep"}){
            auto c=m.init_cache();for(int i=0;i<32;++i)(void)m.step(static_cast<std::uint32_t>(i%256),c,mode);
            auto t0=std::chrono::steady_clock::now();for(int i=0;i<iters;++i)(void)m.step(static_cast<std::uint32_t>((i*17)%256),c,mode);auto t1=std::chrono::steady_clock::now();double s=std::chrono::duration<double>(t1-t0).count();
            std::cout<<mode<<"_tokens_per_s="<<(iters/s)<<"\n";
        }
        std::cout<<"max_rss_kb="<<rss_kb()<<"\n";
    }catch(const std::exception&e){std::cerr<<"ERROR: "<<e.what()<<"\n";return 1;}
}
