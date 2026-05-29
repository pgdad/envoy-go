// Conformance family `exports` — WASI exports surface (env / clock / random).
//
// on_vm_start the guest exercises the three host-observable WASI exports and
// writes each result into shared-data as raw bytes (no SDK string coercion):
//
//   "conformance-exports-env"    = bytes of std::env::var("CONFORMANCE_EXPORTS")
//                                  -> environ_sizes_get / environ_get
//   "conformance-exports-clock"  = SystemTime::now() duration-since-epoch nanos,
//                                  as u64 little-endian -> clock_time_get
//   "conformance-exports-random" = 16 bytes from the raw wasi random_get import
//
// The harness seeds CONFORMANCE_EXPORTS=<known> via WithRootEnv, then reads the
// three shared-data keys via RootVM.GetSharedData and asserts:
//   - env value round-trips byte-faithfully (the deterministic observable),
//   - clock nanos are non-zero,
//   - random buffer is the requested 16 bytes and not all-zero.
use proxy_wasm::hostcalls;
use proxy_wasm::traits::*;
use proxy_wasm::types::*;
use std::time::{SystemTime, UNIX_EPOCH};

// Raw WASI random_get import (wasi_snapshot_preview1.random_get). Declared
// directly so the guest exercises the host's random_get shim without pulling
// in the getrandom crate. Returns a wasi errno (0 == success).
#[link(wasm_import_module = "wasi_snapshot_preview1")]
extern "C" {
    fn random_get(buf: *mut u8, buf_len: usize) -> u16;
}

proxy_wasm::main! {{
    proxy_wasm::set_log_level(LogLevel::Info);
    proxy_wasm::set_root_context(|_| -> Box<dyn RootContext> { Box::new(Root) });
}}

struct Root;
impl Context for Root {}
impl RootContext for Root {
    fn on_vm_start(&mut self, _: usize) -> bool {
        // environ_get / environ_sizes_get via std::env.
        let env_val = std::env::var("CONFORMANCE_EXPORTS").unwrap_or_else(|_| "MISSING".to_string());
        let _ = hostcalls::set_shared_data(
            "conformance-exports-env",
            Some(env_val.as_bytes()),
            Some(0),
        );

        // clock_time_get via SystemTime::now().
        let nanos = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .map(|d| d.as_nanos() as u64)
            .unwrap_or(0);
        let _ = hostcalls::set_shared_data(
            "conformance-exports-clock",
            Some(&nanos.to_le_bytes()),
            Some(0),
        );

        // random_get via the raw WASI import.
        let mut rnd = [0u8; 16];
        let errno = unsafe { random_get(rnd.as_mut_ptr(), rnd.len()) };
        // Stash the buffer regardless; errno 0 means the host filled it.
        let _ = hostcalls::set_shared_data(
            "conformance-exports-random",
            Some(&rnd),
            Some(0),
        );
        let _ = hostcalls::set_shared_data(
            "conformance-exports-random-errno",
            Some(&(errno as u32).to_le_bytes()),
            Some(0),
        );

        true
    }
}
