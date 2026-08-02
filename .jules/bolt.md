# Bolt's Journal

Only critical learnings are logged here to avoid clutter.

## 2025-03-05 - Go Hot Loop BCE and Struct Field Reloading Optimization
**Learning:** In Go, indexing a slice in a hot loop can still incur bounds-check overhead even if BCE assertions are manually written. By replacing slice lookups with a pointer to a fixed-size array (e.g. `*[256]byte`) and using index type `byte`, the compiler statically guarantees no out-of-bounds index can occur, completely eliminating bounds checks. Furthermore, accessing a pointer/slice stored on a receiver struct (e.g. `dr.xorLookup`) forces the compiler to reload the field in every iteration. Lifting the dereference out of the loop (e.g. `lookup := dr.xorLookup`) allows the compiler to load the address into a register once, resulting in an extra ~17.5% speedup.
**Action:** Next time when optimizing a hot indexing/decryption loop, always prefer pointers to fixed-size arrays indexed by matching integer types (like `byte` into `*[256]byte`), and always lift receiver field dereferences out of the loop.

## 2026-07-14 - Go Hot Loop Unrolling and Instruction-Level Parallelism (ILP)
**Learning:** Even after achieving complete Bounds Check Elimination (BCE) in a hot XOR decryption loop, loop overhead (condition checking and incrementing) remains a major performance factor. By unrolling the loop by 8, we significantly reduce loop control instruction overhead and allow the CPU to perform instruction-level parallelism (ILP) on independent XOR operations. Go's compiler still statically proves bounds checks are elided for constant offset expressions on `*[256]byte` indexed by `byte`. This resulted in a ~14.8% decryption speedup.
**Action:** For performance-critical encryption, decryption, or hashing hot loops, unroll the loop by 8 to leverage CPU pipeline parallelism, while ensuring Go's bounds-checking remains fully elided.

## 2026-07-15 - Go Non-Unit Loop Stride BCE with Sub-Slicing
**Learning:** Go compiler's induction variable analysis and bounds-check elimination (BCE) are highly optimized for loops with increment steps of 1 (`i++`), but fail to eliminate bounds checks for unrolled loops with non-unit strides (like `i += 8`), even when a pre-loop assertion like `_ = p[n-1]` is present. To completely eliminate bounds checks on the target slice inside the unrolled loop, we can create a sub-slice `sub := p[i : i+8]` and assert its bounds with `_ = sub[7]`. This reduces the number of slice/array bounds checks from 8 down to exactly 1 per loop iteration, resulting in an additional ~14.6% decryption speedup.
**Action:** When unrolling hot loops with non-unit stride, always use the sub-slicing trick to encapsulate the unrolled block and assert the sub-slice's max index to completely eliminate multi-bounds check overhead.
