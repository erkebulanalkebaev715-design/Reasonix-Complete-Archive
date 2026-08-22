# Reasonix v0.6 architecture/runtime

Мозг остаётся собственным Reasonix v0.4/v0.5 ядром: Selective Pocket State + rare Window Latent Attention + compact shared/routed Sparse Latent Experts + Anchor Mixer + FAST/STANDARD/DEEP depth paths.

## RXM6 zero-copy weight path

`RXM6MMAP` хранит config и тензоры в 64-byte aligned sequential records. Native loader делает `mmap(MAP_PRIVATE)` и сохраняет указатели на payload внутри mapped region. Quantized weights не копируются целиком в отдельные `std::vector`.

Tensor kinds:

- F32: embeddings, norms, biases, scalar/vector parameters;
- INT8_ROW: per-row symmetric INT8 для state/attention/router/projections/head;
- INT4_GROUP: signed 4-bit, 2 weights/byte, group-wise scale (32 weights) для expert MLP matrices в experimental mixed policy.

## Scratch arena phase 1

`NativeReasonix` держит persistent INT8 input scratch, размер которого выбирается по максимальной внутренней ширине модели. Это убирает выделение временного INT8 input buffer на каждый Linear. Полный float activation arena остаётся следующим этапом.

## K/V

Редкий WLA attention использует фиксированный ring buffer. Размер не растёт после `window_size`; при вытеснении нет сдвига всего K/V массива.

## Phone autotune

Phone gate сравнивает два одинаковых smoke graph:

1. all-linear INT8;
2. mixed INT8 + expert INT4.

Mixed разрешается только если:

- его файл меньше;
- deep throughput >= 85% INT8 throughput.

Это специально не даёт считать INT4 улучшением только из-за меньшего файла.

## Mobile-scale fixture

`mobile_s_v06_mixed_BENCH_ONLY.rxm6` использует настоящий граф `mobile_s` (23,574,210 trainable parameters before quantized export), но случайные веса. Он существует только для измерения inference-engine speed/RSS на физическом Android.
