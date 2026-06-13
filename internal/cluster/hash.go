package cluster

import (
	"encoding/binary"
	"math/bits"
	"net"
	"sort"
)

// hash.go provides pure-Go ports of the two hash digests Envoy uses to compute
// consistent-hash load-balancer ring positions and source_ip keys:
//
//   - xxHash64: the XXH64 reference algorithm with seed fixed to 0. This is
//     Envoy's XX_HASH default (source/common/common/hash.h HashUtil::xxHash64,
//     which delegates to the upstream xxhash with seed 0).
//   - murmurHash2: MurmurHash64A (Austin Appleby's canonical public-domain
//     algorithm), the MURMUR_HASH_2 arm. Envoy seeds it with 0xc70f6907
//     (source/common/common/hash.h HashUtil::murmurHash2).
//
// Both digests MUST match the reference byte-for-byte: the Ketama ring positions
// are derived directly from these hashes, so any deviation produces a different
// ring and breaks faithful reproduction of Envoy's host selection.
//
// These are deliberately pure-Go (no cespare/xxhash dependency): adding a new
// go.mod dependency for two internal call sites is unwarranted (D-RH1 /
// AMEND-RH7). Vectors are pinned in hash_test.go from trusted oracles.

// XXH64 prime constants (upstream xxhash reference).
const (
	prime64_1 = 0x9E3779B185EBCA87
	prime64_2 = 0xC2B2AE3D27D4EB4F
	prime64_3 = 0x165667B19E3779F9
	prime64_4 = 0x85EBCA77C2B2AE63
	prime64_5 = 0x27D4EB2F165667C5
)

// xxHash64 computes the XXH64 digest of b with the seed fixed to 0, faithfully
// reproducing Envoy's XX_HASH default. Side-effect-free. Delegates to the
// seeded variant.
func xxHash64(b []byte) uint64 { return xxHash64Seed(b, 0) }

// xxHash64Seed is the XXH64 reference algorithm with a caller-supplied seed —
// the seed-chained header fold (HashHeaderValues) feeds each value's output as
// the next value's seed (source/common/common/hash.cc:7-12). seed==0 reproduces
// xxHash64's incumbent behavior byte-for-byte. Side-effect-free.
func xxHash64Seed(b []byte, seed uint64) uint64 {
	n := len(b)
	var h uint64

	if n >= 32 {
		// 4-lane accumulator stripe loop.
		v1 := seed + prime64_1 + prime64_2
		v2 := seed + prime64_2
		v3 := seed + 0
		v4 := seed - prime64_1

		for len(b) >= 32 {
			v1 = xxRound(v1, binary.LittleEndian.Uint64(b[0:8]))
			v2 = xxRound(v2, binary.LittleEndian.Uint64(b[8:16]))
			v3 = xxRound(v3, binary.LittleEndian.Uint64(b[16:24]))
			v4 = xxRound(v4, binary.LittleEndian.Uint64(b[24:32]))
			b = b[32:]
		}

		h = bits.RotateLeft64(v1, 1) + bits.RotateLeft64(v2, 7) +
			bits.RotateLeft64(v3, 12) + bits.RotateLeft64(v4, 18)

		h = xxMergeRound(h, v1)
		h = xxMergeRound(h, v2)
		h = xxMergeRound(h, v3)
		h = xxMergeRound(h, v4)
	} else {
		h = seed + prime64_5
	}

	h += uint64(n)

	// Remaining-bytes tail.
	for len(b) >= 8 {
		k1 := xxRound(0, binary.LittleEndian.Uint64(b[0:8]))
		h ^= k1
		h = bits.RotateLeft64(h, 27)*prime64_1 + prime64_4
		b = b[8:]
	}
	if len(b) >= 4 {
		h ^= uint64(binary.LittleEndian.Uint32(b[0:4])) * prime64_1
		h = bits.RotateLeft64(h, 23)*prime64_2 + prime64_3
		b = b[4:]
	}
	for _, c := range b {
		h ^= uint64(c) * prime64_5
		h = bits.RotateLeft64(h, 11) * prime64_1
	}

	// Final avalanche.
	h ^= h >> 33
	h *= prime64_2
	h ^= h >> 29
	h *= prime64_3
	h ^= h >> 32
	return h
}

func xxRound(acc, input uint64) uint64 {
	acc += input * prime64_2
	acc = bits.RotateLeft64(acc, 31)
	acc *= prime64_1
	return acc
}

func xxMergeRound(acc, val uint64) uint64 {
	val = xxRound(0, val)
	acc ^= val
	acc = acc*prime64_1 + prime64_4
	return acc
}

// ipOnly returns the bare IP from a "host:port" address, stripping the port
// (AMEND-RH3: Envoy hashes addressAsString() — the IP WITHOUT the port). If
// addr has no port (already a bare IP), it is returned unchanged.
func ipOnly(addr string) string {
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return addr
}

// HashSourceIP returns the ring_hash consistent-hash key for a downstream
// source address "host:port": xxHash64 over the bare client IP (port stripped).
// Exported so producers in other packages (tcpproxy source_ip; the 36.2 HTTP
// route hash_policy) compute the key without reaching cluster's unexported
// xxHash64/ipOnly. ADR-0235/ADR-0236.
func HashSourceIP(addr string) uint64 {
	return xxHash64([]byte(ipOnly(addr)))
}

// HashHeaderValues returns the ring_hash consistent-hash key for an HTTP route
// header hash_policy: the seed-chained XXH64 fold over the BYTE-SORTED header
// values (Envoy HeaderHashMethod — source/common/http/hash_policy.cc:43-77).
// A single value collapses to xxHash64([]byte(value)); an empty list → 0.
// Exported so the http/router producer computes the digest without reaching
// cluster's unexported xxHash64 (the ONE additive 36.2 exported symbol;
// the exported Cluster surface stays byte-stable). ADR-0237.
func HashHeaderValues(values []string) uint64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]string(nil), values...)
	sort.Strings(sorted) // byte-wise std::sort parity
	var seed uint64
	for _, v := range sorted {
		seed = xxHash64Seed([]byte(v), seed)
	}
	return seed
}

// murmurHash2 computes MurmurHash64A (Austin Appleby's canonical public-domain
// MurmurHash2.cpp algorithm) of b with the caller-supplied seed. Envoy's
// MURMUR_HASH_2 arm seeds this with 0xc70f6907. Side-effect-free.
func murmurHash2(b []byte, seed uint64) uint64 {
	const (
		m = 0xc6a4a7935bd1e995
		r = 47
	)
	n := len(b)
	h := seed ^ (uint64(n) * m)

	// Body: process 8-byte blocks.
	for len(b) >= 8 {
		k := binary.LittleEndian.Uint64(b[0:8])
		k *= m
		k ^= k >> r
		k *= m
		h ^= k
		h *= m
		b = b[8:]
	}

	// Tail: remaining 0..7 bytes (fall-through switch in the C reference).
	switch len(b) {
	case 7:
		h ^= uint64(b[6]) << 48
		fallthrough
	case 6:
		h ^= uint64(b[5]) << 40
		fallthrough
	case 5:
		h ^= uint64(b[4]) << 32
		fallthrough
	case 4:
		h ^= uint64(b[3]) << 24
		fallthrough
	case 3:
		h ^= uint64(b[2]) << 16
		fallthrough
	case 2:
		h ^= uint64(b[1]) << 8
		fallthrough
	case 1:
		h ^= uint64(b[0])
		h *= m
	}

	// Final mix.
	h ^= h >> r
	h *= m
	h ^= h >> r
	return h
}
