# Changelog

## v0.6 PHONE-ONLY

- Removed PC from required user workflow.
- Added `RXM6MMAP` project-native model container.
- Added mmap-backed native weight views.
- Added group-wise signed INT4 expert storage + C++ primitive.
- Kept critical matrices in INT8 for mixed policy.
- Added persistent INT8 matvec scratch arena.
- Added one-tap Termux build/correctness/benchmark/autotune script.
- Added automatic copy from Android shared storage to Termux private HOME to avoid `noexec` problems.
- Added real mobile-scale BENCH-ONLY model file based on Reasonix `mobile_s` graph.
- Added RSS, mapped bytes, memory and thermal reporting.
- Added policy gate that can reject INT4 when its speed loss is too large.
- Added v0.6 quant/native regression tests.

### Rejected claim

INT4 is not called an optimization yet. Host measurement: ~11.95% smaller smoke model but only ~60.5% of INT8 deep throughput. The implementation remains experimental until ARM measurements/kernel work justify it.
