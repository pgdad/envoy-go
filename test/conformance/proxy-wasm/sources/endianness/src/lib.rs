// Conformance family `endianness` — LE multi-byte marshalling across the ABI.
//
// on_vm_start the guest writes two known integers as their little-endian byte
// representation into shared-data (a raw-bytes buffer hostcall, so no SDK
// string coercion intervenes):
//   "conformance-le-u32" = 0x01020304u32.to_le_bytes() = [0x04,0x03,0x02,0x01]
//   "conformance-le-u64" = 0x0102030405060708u64.to_le_bytes()
//                          = [0x08,0x07,0x06,0x05,0x04,0x03,0x02,0x01]
//
// The harness reads the raw bytes via RootVM.GetSharedData + asserts the exact
// LE byte ordering, proving guest<->host buffer marshalling preserves byte
// order (wazero + the envoy-go host run little-endian; the buffer transfer is
// an identity copy, matching upstream usesWasmByteOrder == false).
use proxy_wasm::hostcalls;
use proxy_wasm::traits::*;
use proxy_wasm::types::*;

proxy_wasm::main! {{
    proxy_wasm::set_log_level(LogLevel::Info);
    proxy_wasm::set_root_context(|_| -> Box<dyn RootContext> { Box::new(Root) });
}}

struct Root;
impl Context for Root {}
impl RootContext for Root {
    fn on_vm_start(&mut self, _: usize) -> bool {
        let u32_le = 0x0102_0304u32.to_le_bytes();
        let u64_le = 0x0102_0304_0506_0708u64.to_le_bytes();
        let _ = hostcalls::set_shared_data("conformance-le-u32", Some(&u32_le), Some(0));
        let _ = hostcalls::set_shared_data("conformance-le-u64", Some(&u64_le), Some(0));
        true
    }
}
