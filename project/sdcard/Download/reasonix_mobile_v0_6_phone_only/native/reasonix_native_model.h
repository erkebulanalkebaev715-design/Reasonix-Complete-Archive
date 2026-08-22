#pragma once
#include <cstddef>
#include <cstdint>
#include <memory>
#include <string>
#include <unordered_map>
#include <vector>

namespace reasonix {

enum class TensorKind : std::uint8_t { F32 = 0, INT8_ROW = 1, INT4_GROUP = 2 };

struct MappedRegion;

struct Tensor {
    TensorKind kind{};
    std::vector<std::uint32_t> dims;
    std::uint32_t group_size = 0;
    std::uint32_t scale_count = 0;
    std::uint64_t payload_bytes = 0;
    const float* f32 = nullptr;
    const float* scales = nullptr;
    const std::int8_t* q8 = nullptr;
    const std::uint8_t* q4 = nullptr;
    std::size_t numel() const;
};

struct NativeConfig {
    std::uint32_t vocab_size{}, d_model{}, n_layers{}, d_state{}, d_latent{};
    std::uint32_t n_experts{}, expert_ff{}, shared_expert_ff{}, attn_every{};
    std::uint32_t n_heads{}, attn_head_dim{}, attn_value_dim{}, window_size{}, anchor_interval{};
    float fast_depth_fraction{}, standard_depth_fraction{};
};

struct NativeWeights {
    NativeConfig cfg;
    std::unordered_map<std::string, Tensor> tensors;
    std::shared_ptr<MappedRegion> mapping;
    std::size_t mapped_bytes = 0;
    static NativeWeights load(const std::string& path);
    const Tensor& at(const std::string& name) const;
};

struct LayerCacheNative {
    std::vector<float> state;
    std::vector<float> keys;
    std::vector<float> values;
    std::uint32_t attn_tokens = 0;
    std::uint32_t attn_start = 0;
};
struct ModelCacheNative { std::vector<LayerCacheNative> layers; };

class NativeReasonix {
public:
    explicit NativeReasonix(NativeWeights w);
    ModelCacheNative init_cache() const;
    std::vector<float> step(std::uint32_t token, ModelCacheNative& cache, const std::string& mode = "deep") const;
    std::vector<std::uint32_t> greedy(const std::vector<std::uint32_t>& prompt, std::uint32_t new_tokens, const std::string& mode = "deep") const;
    const NativeConfig& config() const { return w_.cfg; }
    std::size_t mapped_bytes() const { return w_.mapped_bytes; }
private:
    NativeWeights w_;
    mutable std::vector<std::int8_t> qscratch_; // persistent per-model matvec scratch arena
    std::uint32_t depth_for_mode(const std::string& mode) const;
};

} // namespace reasonix
