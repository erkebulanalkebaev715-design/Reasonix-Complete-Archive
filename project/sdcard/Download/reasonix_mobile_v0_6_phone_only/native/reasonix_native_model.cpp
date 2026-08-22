#include "reasonix_native_model.h"
#include "reasonix_matvec.h"
#include <algorithm>
#include <cmath>
#include <cstring>
#include <fcntl.h>
#include <stdexcept>
#include <sys/mman.h>
#include <sys/stat.h>
#include <unistd.h>

namespace reasonix {
struct MappedRegion {
    int fd=-1; std::size_t size=0; const std::uint8_t* base=nullptr;
    ~MappedRegion(){ if(base && base!=MAP_FAILED) munmap(const_cast<std::uint8_t*>(base),size); if(fd>=0) close(fd); }
};
namespace {
struct Reader {
    const std::uint8_t* p; const std::uint8_t* end;
    template<class T>T pod(){ if(static_cast<std::size_t>(end-p)<sizeof(T))throw std::runtime_error("truncated RXM6");T v;std::memcpy(&v,p,sizeof(T));p+=sizeof(T);return v; }
    void bytes(void*out,std::size_t n){if(static_cast<std::size_t>(end-p)<n)throw std::runtime_error("truncated RXM6");std::memcpy(out,p,n);p+=n;}
    void align(std::size_t a=64){auto off=static_cast<std::size_t>(p-base());auto pad=(-off)&(a-1);if(static_cast<std::size_t>(end-p)<pad)throw std::runtime_error("bad RXM6 align");p+=pad;}
    const std::uint8_t* base0=nullptr; const std::uint8_t* base() const{return base0;}
};
float sigmoid(float x){return 1.0f/(1.0f+std::exp(-x));}
float silu(float x){return x*sigmoid(x);} 
float softplus(float x){if(x>20)return x;if(x<-20)return std::exp(x);return std::log1p(std::exp(x));}
void rmsnorm(const std::vector<float>&x,const Tensor&w,std::vector<float>&out){if(w.kind!=TensorKind::F32||w.numel()!=x.size())throw std::runtime_error("bad rmsnorm");double ss=0;for(float v:x)ss+=double(v)*v;float inv=1.0f/std::sqrt(float(ss/x.size())+1e-6f);out.resize(x.size());for(std::size_t i=0;i<x.size();++i)out[i]=x[i]*inv*w.f32[i];}
void linear(const Tensor&w,const Tensor&b,const std::vector<float>&x,std::vector<float>&out,std::vector<std::int8_t>&scratch){
    if(w.dims.size()!=2||w.dims[1]!=x.size()) throw std::runtime_error("bad linear shape");
    const std::size_t rows=w.dims[0],cols=w.dims[1]; out.assign(rows,0.0f);
    if(w.kind==TensorKind::INT8_ROW){if(scratch.size()<cols)scratch.resize(cols);matvec_s8_per_row(w.q8,w.scales,x.data(),out.data(),rows,cols,scratch.data());}
    else if(w.kind==TensorKind::INT4_GROUP){matvec_s4_group_per_row(w.q4,w.scales,x.data(),out.data(),rows,cols,w.group_size);}
    else throw std::runtime_error("linear weight must be quantized");
    if(b.kind==TensorKind::F32&&b.f32){if(b.numel()!=rows)throw std::runtime_error("bad bias");for(std::size_t i=0;i<rows;++i)out[i]+=b.f32[i];}
}
void linear_nb(const Tensor&w,const std::vector<float>&x,std::vector<float>&out,std::vector<std::int8_t>&scratch){Tensor none{};linear(w,none,x,out,scratch);}
void add_inplace(std::vector<float>&a,const std::vector<float>&b){if(a.size()!=b.size())throw std::runtime_error("add mismatch");for(std::size_t i=0;i<a.size();++i)a[i]+=b[i];}
void softmax(std::vector<float>&x){float m=*std::max_element(x.begin(),x.end());double s=0;for(float&v:x){v=std::exp(v-m);s+=v;}float inv=1.0f/float(s);for(float&v:x)v*=inv;}
void tiny_glu(const NativeWeights&w,const std::string&prefix,const std::vector<float>&z,std::vector<float>&out,std::vector<std::int8_t>&scratch){std::vector<float>g,u,h;linear_nb(w.at(prefix+".g.weight"),z,g,scratch);linear_nb(w.at(prefix+".u.weight"),z,u,scratch);h.resize(g.size());for(std::size_t i=0;i<g.size();++i)h[i]=silu(g[i])*u[i];linear_nb(w.at(prefix+".down.weight"),h,out,scratch);}
}

std::size_t Tensor::numel()const{std::size_t n=1;for(auto d:dims)n*=d;return n;}
const Tensor&NativeWeights::at(const std::string&name)const{auto it=tensors.find(name);if(it==tensors.end())throw std::runtime_error("missing tensor: "+name);return it->second;}
NativeWeights NativeWeights::load(const std::string&path){
    auto map=std::make_shared<MappedRegion>();map->fd=open(path.c_str(),O_RDONLY);if(map->fd<0)throw std::runtime_error("cannot open RXM6");struct stat st{};if(fstat(map->fd,&st)!=0||st.st_size<=0)throw std::runtime_error("cannot stat RXM6");map->size=static_cast<std::size_t>(st.st_size);map->base=static_cast<const std::uint8_t*>(mmap(nullptr,map->size,PROT_READ,MAP_PRIVATE,map->fd,0));if(map->base==MAP_FAILED)throw std::runtime_error("mmap RXM6 failed");
    Reader r{map->base,map->base+map->size};r.base0=map->base;char magic[8];r.bytes(magic,8);const char expect[8]={'R','X','M','6','M','M','A','P'};if(std::memcmp(magic,expect,8)!=0)throw std::runtime_error("bad RXM6 magic");if(r.pod<std::uint32_t>()!=1)throw std::runtime_error("unsupported RXM6 version");
    NativeWeights out;out.mapping=map;out.mapped_bytes=map->size;auto&c=out.cfg;c.vocab_size=r.pod<std::uint32_t>();c.d_model=r.pod<std::uint32_t>();c.n_layers=r.pod<std::uint32_t>();c.d_state=r.pod<std::uint32_t>();c.d_latent=r.pod<std::uint32_t>();c.n_experts=r.pod<std::uint32_t>();c.expert_ff=r.pod<std::uint32_t>();c.shared_expert_ff=r.pod<std::uint32_t>();c.attn_every=r.pod<std::uint32_t>();c.n_heads=r.pod<std::uint32_t>();c.attn_head_dim=r.pod<std::uint32_t>();c.attn_value_dim=r.pod<std::uint32_t>();c.window_size=r.pod<std::uint32_t>();c.anchor_interval=r.pod<std::uint32_t>();c.fast_depth_fraction=r.pod<float>();c.standard_depth_fraction=r.pod<float>();auto nt=r.pod<std::uint32_t>();
    for(std::uint32_t ti=0;ti<nt;++ti){auto nl=r.pod<std::uint16_t>();std::string name(nl,'\0');r.bytes(name.data(),nl);Tensor t;t.kind=static_cast<TensorKind>(r.pod<std::uint8_t>());auto nd=r.pod<std::uint8_t>();t.dims.resize(nd);for(auto&d:t.dims)d=r.pod<std::uint32_t>();t.group_size=r.pod<std::uint32_t>();t.scale_count=r.pod<std::uint32_t>();t.payload_bytes=r.pod<std::uint64_t>();r.align();if(t.scale_count){if(static_cast<std::size_t>(r.end-r.p)<std::size_t(t.scale_count)*sizeof(float))throw std::runtime_error("truncated scales");t.scales=reinterpret_cast<const float*>(r.p);r.p+=std::size_t(t.scale_count)*sizeof(float);}r.align();if(static_cast<std::size_t>(r.end-r.p)<t.payload_bytes)throw std::runtime_error("truncated payload");if(t.kind==TensorKind::F32)t.f32=reinterpret_cast<const float*>(r.p);else if(t.kind==TensorKind::INT8_ROW)t.q8=reinterpret_cast<const std::int8_t*>(r.p);else if(t.kind==TensorKind::INT4_GROUP)t.q4=r.p;else throw std::runtime_error("bad tensor kind");r.p+=t.payload_bytes;r.align();out.tensors.emplace(std::move(name),std::move(t));}
    return out;
}
NativeReasonix::NativeReasonix(NativeWeights w):w_(std::move(w)){if(!w_.cfg.vocab_size||!w_.cfg.d_model||!w_.cfg.n_layers)throw std::runtime_error("invalid config");qscratch_.resize(std::max({w_.cfg.d_model,w_.cfg.d_state,w_.cfg.d_latent,w_.cfg.expert_ff,w_.cfg.shared_expert_ff,std::uint32_t(w_.cfg.n_heads*w_.cfg.attn_value_dim)}));}
ModelCacheNative NativeReasonix::init_cache()const{ModelCacheNative c;c.layers.resize(w_.cfg.n_layers);for(std::size_t i=0;i<c.layers.size();++i){c.layers[i].state.assign(w_.cfg.d_state,0);if((i+1)%w_.cfg.attn_every==0){c.layers[i].keys.assign(std::size_t(w_.cfg.window_size)*w_.cfg.n_heads*w_.cfg.attn_head_dim,0);c.layers[i].values.assign(std::size_t(w_.cfg.window_size)*w_.cfg.n_heads*w_.cfg.attn_value_dim,0);}}return c;}
std::uint32_t NativeReasonix::depth_for_mode(const std::string&m)const{if(m=="deep")return w_.cfg.n_layers;float f=m=="fast"?w_.cfg.fast_depth_fraction:m=="standard"?w_.cfg.standard_depth_fraction:-1;if(f<=0)throw std::runtime_error("unknown mode");auto d=std::uint32_t(std::lround(w_.cfg.n_layers*f));return std::max<std::uint32_t>(1,std::min(w_.cfg.n_layers,d));}
std::vector<float> NativeReasonix::step(std::uint32_t token,ModelCacheNative&cache,const std::string&mode)const{
    const auto&cfg=w_.cfg;if(token>=cfg.vocab_size)throw std::runtime_error("token range");const auto&emb=w_.at("embed.weight");if(emb.kind!=TensorKind::F32||emb.dims.size()!=2)throw std::runtime_error("embed f32");std::vector<float>x(cfg.d_model),anchor(cfg.d_model);const float*erow=emb.f32+std::size_t(token)*cfg.d_model;std::copy(erow,erow+cfg.d_model,x.begin());anchor=x;auto depth=depth_for_mode(mode);
    for(std::uint32_t li=0;li<depth;++li){auto&lc=cache.layers[li];auto p="layers."+std::to_string(li)+".";std::vector<float>z,keep,write,read,gate,y;rmsnorm(x,w_.at(p+"state.norm.weight"),z);linear(w_.at(p+"state.to_keep.weight"),w_.at(p+"state.to_keep.bias"),z,keep,qscratch_);linear_nb(w_.at(p+"state.to_write.weight"),z,write,qscratch_);for(std::size_t j=0;j<cfg.d_state;++j){keep[j]=sigmoid(keep[j]);write[j]=std::tanh(write[j]);lc.state[j]=keep[j]*lc.state[j]+(1-keep[j])*write[j];}linear_nb(w_.at(p+"state.to_read.weight"),lc.state,read,qscratch_);linear(w_.at(p+"state.to_gate.weight"),w_.at(p+"state.to_gate.bias"),z,gate,qscratch_);y.resize(cfg.d_model);for(std::size_t j=0;j<cfg.d_model;++j)y[j]=read[j]*sigmoid(gate[j]);add_inplace(x,y);
        if((li+1)%cfg.attn_every==0){auto ap=p+"attention.";std::vector<float>az,q,k,v;rmsnorm(x,w_.at(ap+"norm.weight"),az);linear_nb(w_.at(ap+"q.weight"),az,q,qscratch_);linear_nb(w_.at(ap+"k.weight"),az,k,qscratch_);linear_nb(w_.at(ap+"v.weight"),az,v,qscratch_);std::size_t ks=cfg.n_heads*cfg.attn_head_dim,vs=cfg.n_heads*cfg.attn_value_dim;std::uint32_t slot;if(lc.attn_tokens<cfg.window_size){slot=(lc.attn_start+lc.attn_tokens)%cfg.window_size;++lc.attn_tokens;}else{slot=lc.attn_start;lc.attn_start=(lc.attn_start+1)%cfg.window_size;}std::copy(k.begin(),k.end(),lc.keys.begin()+std::size_t(slot)*ks);std::copy(v.begin(),v.end(),lc.values.begin()+std::size_t(slot)*vs);std::vector<float>mixed(vs,0);const auto&slope=w_.at(ap+"recency_log_slope");for(std::uint32_t h=0;h<cfg.n_heads;++h){std::vector<float>scores(lc.attn_tokens);for(std::uint32_t t=0;t<lc.attn_tokens;++t){auto phys=(lc.attn_start+t)%cfg.window_size;double dot=0;auto ko=(std::size_t(phys)*cfg.n_heads+h)*cfg.attn_head_dim,qo=std::size_t(h)*cfg.attn_head_dim;for(std::uint32_t d=0;d<cfg.attn_head_dim;++d)dot+=q[qo+d]*lc.keys[ko+d];float dist=float(lc.attn_tokens-1-t);scores[t]=float(dot/std::sqrt(double(cfg.attn_head_dim)))-softplus(slope.f32[h])*dist;}softmax(scores);auto mo=std::size_t(h)*cfg.attn_value_dim;for(std::uint32_t t=0;t<lc.attn_tokens;++t){auto phys=(lc.attn_start+t)%cfg.window_size; auto vo=(std::size_t(phys)*cfg.n_heads+h)*cfg.attn_value_dim;for(std::uint32_t d=0;d<cfg.attn_value_dim;++d)mixed[mo+d]+=scores[t]*lc.values[vo+d];}}std::vector<float>ao,ag;linear_nb(w_.at(ap+"out.weight"),mixed,ao,qscratch_);linear(w_.at(ap+"gate.weight"),w_.at(ap+"gate.bias"),az,ag,qscratch_);for(std::size_t j=0;j<x.size();++j)x[j]+=ao[j]*sigmoid(ag[j]);}
        auto ep=p+"experts.";std::vector<float>ez,latent,router;rmsnorm(x,w_.at(ep+"norm.weight"),ez);linear_nb(w_.at(ep+"down.weight"),ez,latent,qscratch_);linear(w_.at(ep+"router.weight"),w_.at(ep+"router.bias"),latent,router,qscratch_);softmax(router);auto chosen=std::size_t(std::max_element(router.begin(),router.end())-router.begin());std::vector<float>shared,routed;tiny_glu(w_,ep+"shared",latent,shared,qscratch_);tiny_glu(w_,ep+"experts."+std::to_string(chosen),latent,routed,qscratch_);for(std::size_t j=0;j<latent.size();++j)shared[j]+=routed[j]*router[chosen];std::vector<float>eup;linear_nb(w_.at(ep+"up.weight"),shared,eup,qscratch_);add_inplace(x,eup);
        if((li+1)%cfg.anchor_interval==0){const auto&logit=w_.at(p+"anchor.logit");for(std::size_t j=0;j<x.size();++j){float a=sigmoid(logit.f32[j]);x[j]=a*x[j]+(1-a)*anchor[j];}anchor=x;}
    }
    std::vector<float>fn,logits;rmsnorm(x,w_.at("final_norm.weight"),fn);linear_nb(w_.at("lm_head.weight"),fn,logits,qscratch_);return logits;
}
std::vector<std::uint32_t> NativeReasonix::greedy(const std::vector<std::uint32_t>&prompt,std::uint32_t n,const std::string&mode)const{if(prompt.empty())throw std::runtime_error("empty prompt");auto c=init_cache();std::vector<float>logits;for(auto t:prompt)logits=step(t,c,mode);std::vector<std::uint32_t>out;for(std::uint32_t i=0;i<n;++i){auto it=std::max_element(logits.begin(),logits.end());auto tok=std::uint32_t(it-logits.begin());out.push_back(tok);logits=step(tok,c,mode);}return out;}
} // namespace reasonix
