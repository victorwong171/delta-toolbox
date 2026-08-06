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

## 2026-07-16 - Bundled Pooling of Dependent Allocations in Go
**Learning:** Pooling a reusable receiver structure (like `gzip.Reader`) via `sync.Pool` can still incur hidden heap allocations if it requires other dependent structures (like `bytes.Reader` from `bytes.NewReader(...)`) to be passed to its `Reset(...)` method on every call. This occurs because passing a new pointer forces heap escape. By bundling both `*gzip.Reader` and `*bytes.Reader` in a wrapper struct (`pooledReader`) and pooling them together, we avoid all intermediate allocations, reducing `Decompress` heap allocations from 3 allocs/op to exactly 1 alloc/op (a 66.7% reduction, leaving only the returned slice allocation).
**Action:** Always bundle dependent helper objects together in a custom wrapper struct when implementing reusable resource pools in Go to eliminate escape-to-heap allocation overhead.

## 2026-07-17 - Bounds-Check Free Sliced Range Cleanup Loop
**Learning:** Cleanup/fallback loops handling remaining bytes of an unrolled loop often trigger bounds check warnings if we index with variables updated across different loops. By sub-slicing the remainder (e.g., `rem := p[i:]`) and using a standard `range` iteration `for j := range rem`, the Go compiler statically guarantees 100% bounds-check free indexing inside the cleanup loop.
**Action:** Always slice the remainder of unrolled loops and iterate over the sub-slice using `range` to eliminate bounds checks on leftover elements.

## 2026-07-18 - High-Frequency Large Buffer Allocation Prevention with sync.Pool
**Learning:** Allocating large buffers (2MB-4MB) dynamically in high-frequency operations like MD5 file hashing creates severe GC (Garbage Collector) pressure. By pooling pointers to these slices (`*[]byte`) using a `sync.Pool`, we can completely bypass dynamic heap allocations. Pointers prevent slice headers from escaping to the heap during `interface{}` boxing, resulting in 0 allocations for the pool itself. Slices retrieved from the pool can be safely down-sliced to match smaller buffer requirements. This resulted in an over 96% reduction in memory allocations for MD5 calculations.
**Action:** Always use a `sync.Pool` storing pointers to large slices (`*[]byte`) for high-frequency or heavy-load buffer requirements, and down-slice the retrieved buffer as needed.
