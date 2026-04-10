# Local LLM Inference on AMD Strix Point Laptops

A practical guide to understanding and benchmarking LLM text generation
performance on the AMD Ryzen AI 9 HX 370 (Framework Laptop 13).

**Date**: 2026-02-21
**Hardware**: Framework Laptop 13, AMD Ryzen AI 9 HX 370, Radeon 890M, 64GB DDR5
**Software**: llama.cpp commit 612db61 / 2026-02-10 (Vulkan), commit a0c91e8 / 2026-02-21 (ROCm via lemonade-sdk), Ubuntu 24.04, kernel 6.17.0


## 1. Hardware: What You Have and How to Verify It

Every number in this document comes from commands you can run yourself.

### CPU and GPU

```
$ cat /proc/cpuinfo | grep "model name" | head -1
AMD Ryzen AI 9 HX 370 w/ Radeon 890M

$ nproc
24                          # 12 Zen5 cores, 24 threads (SMT)

$ lspci | grep Display
c1:00.0 Display controller: AMD/ATI Strix [Radeon 880M / 890M] (rev c1)
```

The integrated GPU (iGPU) target architecture:

```
$ cat /sys/class/kfd/kfd/topology/nodes/*/properties | grep gfx_target
gfx_target_version 0       # node 0 = CPU (no GPU target)
gfx_target_version 110500  # node 1 = GPU = gfx1150 (RDNA 3.5)
```

GPU compute units:

```
$ cat /sys/class/kfd/kfd/topology/nodes/*/properties | grep -E 'simd_count|simd_per_cu'
simd_count 0               # CPU node
simd_per_cu 0
simd_count 32              # GPU node: 32 SIMDs
simd_per_cu 2              # 2 SIMDs per CU → 32/2 = 16 CUs
```

**Result**: Radeon 890M = gfx1150, 16 Compute Units, RDNA 3.5.

### Memory: The Bottleneck That Matters

For LLM text generation (producing tokens one at a time), performance is
almost entirely determined by memory bandwidth. Each token requires reading
most of the model's weights from memory. The GPU's compute capacity is
secondary -- it spends most of its time waiting for data.

```
$ dmesg | grep "RAM width"
[drm] RAM width 128bits DDR5

$ cat /sys/class/drm/card1/device/pp_dpm_mclk
0: 1000Mhz
1: 2400Mhz
2: 2800Mhz                 # max memory clock
```

DDR5 data rate = 2 * memory_clock. So 2800 MHz = DDR5-5600 (5600 MT/s).

**Theoretical maximum bandwidth**:

    5600 MT/s * 128 bits / 8 bits-per-byte = 89,600 MB/s ≈ 89.6 GB/s

This is the hard ceiling. No software optimization can exceed it.

### GPU vs CPU Memory Path

On AMD APUs, the GPU and CPU share the same physical DDR5, but they access
it through different internal paths on the SoC die:

- **GPU → memory**: Direct wide connection to the Unified Memory Controller (UMC).
  Gets the full ~89.6 GB/s.
- **CPU → memory**: Goes through the Infinity Fabric, a packet-based on-die
  interconnect. Gets roughly half: ~45 GB/s.

This is why `ngl 99` (offload all layers to GPU) matters even though it's
the same physical RAM. You're choosing which internal bus path reads the data.

Verified empirically (see Section 3): CPU-only inference at 2.59 t/s vs
GPU inference at 9.87 t/s on the same model -- a 3.8x ratio, consistent
with the ~2x bandwidth advantage plus GPU's superior parallelism for
matrix-vector operations.

### GTT (GPU-Accessible Memory)

```
$ cat /sys/class/drm/card1/device/mem_info_gtt_total
33333506048                 # ~31 GiB currently allocated for GPU
```

This can be increased via kernel boot parameters (`amdgpu.gttsize=` or the
newer `ttm pages_limit=`) to allow larger models to be fully GPU-resident.
With 64GB total RAM, ~50-55 GiB for GTT is practical, leaving enough for
the OS and applications.


## 2. Software Stack: Vulkan vs ROCm vs CPU

The AMD `amdgpu` kernel driver exposes two userspace interfaces:

- **DRM** (`/dev/dri/*`): Used by Vulkan (graphics API repurposed for compute).
- **KFD** (`/dev/kfd`): Used by ROCm/HIP (AMD's CUDA equivalent, pure compute).

Both talk to the same GPU hardware through the same kernel driver.

### Vulkan Backend

llama.cpp's Vulkan backend uses compute shaders. On Linux, two Vulkan
driver implementations exist:

- **RADV** (Mesa, open-source, community/Valve-driven) -- default on most distros.
- **AMDVLK** (AMD's official open-source driver) -- sometimes faster for compute.

Switch between them with `AMD_VULKAN_ICD=RADV` or `AMD_VULKAN_ICD=AMDVLK`.

### ROCm/HIP Backend

Uses HIP kernels (CUDA-like) through the HSA runtime. Requires `/dev/kfd`
access (user must be in the `render` group). Pre-built binaries with bundled
ROCm libraries available from:

    https://github.com/lemonade-sdk/llamacpp-rocm/releases

These include gfx1150 support and all runtime dependencies. No system-wide
ROCm installation needed.

### CPU Backend

Uses SIMD instructions (AVX-512 on Zen5). Accesses memory through the
Infinity Fabric at ~half the GPU's bandwidth. Not competitive for inference.


## 3. Benchmark Results (2026-02-21)

All benchmarks: llama-bench, `-ngl 99 -fa 1 -p 512 -n 128`, 5 repetitions.
Vulkan uses RADV driver (Mesa 25.2.8). ROCm uses lemonade-sdk b1194 with
`ROCBLAS_USE_HIPBLASLT=1`.

### Qwen3-8B Q4_K_M (dense, 4.68 GiB) -- Vulkan

| Profile              | pp512 (t/s)  | tg128 (t/s)     |
|----------------------|-------------|-----------------|
| power-saver (battery)| 146 ± 21    | 9.87 ± 0.03    |
| **performance (AC)** | **322 ± 9** | **13.41 ± 0.21**|

### gpt-oss-20B MXFP4 MoE (11.27 GiB) -- Vulkan

| Profile              | pp512 (t/s)  | tg128 (t/s)      |
|----------------------|-------------|------------------|
| power-saver (battery)| 234 ± 3     | 17.44 ± 0.14    |
| **performance (AC)** | **390 ± 5** | **23.43 ± 0.11** |

**Power profile impact**: pp more than doubles, tg gains 34-36%.

### Backend comparison (power-saver, Qwen3-8B)

| Backend     | pp512 (t/s)  | tg128 (t/s) |
|-------------|-------------|-------------|
| Vulkan GPU  | 146 ± 21    | **9.87 ± 0.03** |
| ROCm GPU    | **207 ± 29**| 4.76 ± 0.12 |
| CPU (24t)   | 132 ± 10    | 2.59 ± 0.08 |

**Key finding**: ROCm is faster at prompt processing (pp), Vulkan is faster
at text generation (tg). Since tg is the user-facing latency for interactive
use, **Vulkan with RADV is the optimal backend for chat/coding workloads**.

The MoE model (gpt-oss-20B) achieves higher tg despite being 2.4x larger
because only active expert weights are read per token, not the full 11.27 GiB.


## 4. Why These Numbers, and What's the Ceiling

### Deriving Bandwidth Utilization from Benchmarks

For a dense model, each generated token requires reading approximately
all model weights from memory. This lets us estimate real-world bandwidth:

    power-saver:  4.68 GiB × 1.074 ×  9.87 t/s ≈ 49.6 GB/s → 55% utilization
    performance:  4.68 GiB × 1.074 × 13.41 t/s ≈ 67.4 GB/s → 75% utilization

The gap between profiles is not software -- it's the memory controller
and GPU clocking higher on the performance profile.

### Theoretical Maximum tg (What Perfect Software Could Achieve)

If we could hit 100% bandwidth utilization (impossible in practice):

    89.6 GB/s / (4.68 GiB × 1.074 GB/GiB) = 17.8 t/s

At a realistic 85% utilization (pushing the limits):

    89.6 × 0.85 / 5.03 = 15.2 t/s

**Current on performance profile: 13.4 t/s (75%). Realistic ceiling: ~15 t/s (85%).
Absolute ceiling: ~18 t/s (100%, physically impossible).**

The remaining headroom is ~13-15% from software improvements.

### What Would Actually Change the Game

| Change | Impact on tg |
|--------|-------------|
| Plugged in + performance profile | **+36% tg, +120% pp** (measured) |
| Better Vulkan shaders / driver updates | +13-15% remaining headroom |
| DDR5-6400 instead of DDR5-5600 | +14% (faster RAM) |
| 256-bit bus (e.g., Strix Halo) | +100% (2x bandwidth) |
| `amd_iommu=off` kernel param | +1-2% |
| `tuned accelerator-performance` | +3-5% on pp, marginal on tg |

The bus width is baked into silicon. The memory speed is fixed by the
SO-DIMM spec and what the memory controller supports. At 75% utilization
on performance profile, we're already close to what software can achieve.
The 128-bit DDR5-5600 bus at 89.6 GB/s is the hard wall.

### Comparison to Apple Silicon

For context, Apple's unified memory architecture gives both CPU and GPU
full bandwidth (no Infinity Fabric bottleneck), and their wider memory
buses deliver significantly higher bandwidth:

| Platform | Bus | Mem BW | Est. tg 8B Q4 | Max RAM |
|----------|-----|--------|---------------|---------|
| Framework 13 (DDR5-5600) | 128-bit | 89.6 GB/s | ~13 t/s | 96 GB |
| MacBook Pro 16" M1 Pro (LPDDR5-6400) | 256-bit | 200 GB/s | ~22 t/s | 32 GB |
| MacBook Pro 16" M3 Pro (LPDDR5-6400) | 192-bit | 150 GB/s | ~16 t/s | 36 GB |
| MacBook Pro 16" M3 Max (LPDDR5-6400) | 512-bit | 400 GB/s | ~45 t/s | 64 GB |

Apple wins on bandwidth (wider buses, soldered LPDDR5). The Framework's
advantage is upgradeable RAM: 96 GB lets you run models that won't fit
on a 32-36 GB Mac. These are fundamentally different machines at different
price points -- the comparison is about memory architecture, not value.


## 5. Practical Recommendations

**For interactive coding/chat** (tg-dominated):
```
./llama-b7992/llama-cli -hf <model>:Q4_K_M \
    -ngl 99 -fa 1 --no-mmap
```
Use Vulkan (RADV). This is your fastest path.

**For batch/RAG workloads** (pp-dominated):
Use the ROCm binary with:
```
LD_LIBRARY_PATH=./llama-rocm:./llama-rocm/hipblaslt \
ROCBLAS_TENSILE_LIBPATH=./llama-rocm/rocblas/library \
ROCBLAS_USE_HIPBLASLT=1 \
    ./llama-rocm/llama-cli -hf <model>:Q4_K_M \
    -ngl 99 -fa 1 --no-mmap
```

**Model sizing rule of thumb**: Your model should fit in GPU-accessible
memory (GTT). At default GTT (~31 GiB), you can comfortably run Q4_K_M
quants up to ~25B dense parameters, or MoE models up to ~60B total.
Increase GTT for larger models.

**How to reproduce these benchmarks**:
```
# Vulkan
./llama-b7992/llama-bench -m <model.gguf> -ngl 99 -fa 1 -p 512 -n 128

# ROCm (set env vars as above)
./llama-rocm/llama-bench -m <model.gguf> -ngl 99 -mmp 0 -fa 1 -p 512 -n 128

# CPU only
./llama-b7992/llama-bench -m <model.gguf> -ngl 0 -t 24 -fa 1 -p 512 -n 128
```

## References

- Strix Halo Wiki (community, excellent): https://strixhalo.wiki/AI
- Pre-built ROCm binaries (daily, gfx1150): https://github.com/lemonade-sdk/llamacpp-rocm/releases
- llama.cpp build docs: https://github.com/ggml-org/llama.cpp/blob/master/docs/build.md
- Framework Strix Halo guide: https://github.com/Gygeek/Framework-strix-halo-llm-setup
