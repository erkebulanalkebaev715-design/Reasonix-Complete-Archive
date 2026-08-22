#include "reasonix_native_model.h"
#include <algorithm>
#include <cstdlib>
#include <iomanip>
#include <iostream>
#include <sstream>
#include <string>
#include <vector>

static std::vector<std::uint32_t> parse_tokens(const std::string& s){
    std::vector<std::uint32_t> v; std::stringstream ss(s); std::string item;
    while(std::getline(ss,item,',')) if(!item.empty()) v.push_back(static_cast<std::uint32_t>(std::stoul(item)));
    return v;
}

int main(int argc,char**argv){
    if(argc<4){ std::cerr<<"usage: reasonix-native <model.rxm> <comma_tokens> <new_tokens> [mode]\n"; return 2; }
    try{
        auto w=reasonix::NativeWeights::load(argv[1]); reasonix::NativeReasonix m(std::move(w)); auto prompt=parse_tokens(argv[2]); const auto n=static_cast<std::uint32_t>(std::stoul(argv[3]));
        const std::string mode = argc >= 5 ? argv[4] : "deep";
        auto cache=m.init_cache(); std::vector<float> logits; for(auto t:prompt) logits=m.step(t,cache,mode);
        std::vector<std::uint32_t> greedy;
        for(std::uint32_t i=0;i<n;++i){ auto it=std::max_element(logits.begin(),logits.end()); auto tok=static_cast<std::uint32_t>(it-logits.begin()); greedy.push_back(tok); logits=m.step(tok,cache,mode); }
        std::cout<<"GREEDY="; for(std::size_t i=0;i<greedy.size();++i){ if(i)std::cout<<','; std::cout<<greedy[i]; } std::cout<<"\n";
        std::cout<<"LOGITS="<<std::setprecision(9); for(std::size_t i=0;i<logits.size();++i){ if(i)std::cout<<','; std::cout<<logits[i]; } std::cout<<"\n";
    }catch(const std::exception&e){ std::cerr<<"ERROR: "<<e.what()<<"\n"; return 1; }
}
