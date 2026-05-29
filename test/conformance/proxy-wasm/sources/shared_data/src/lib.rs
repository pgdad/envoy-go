// Conformance family `shared_data` — canonical CAS round-trip in on_vm_start.
//
// Sequence (key = "conformance-shared-key"):
//   1. set(key, "v1", cas=0)         -> Ok   (unconditional first write; cas -> 1)
//   2. get(key)                      -> ("v1", cas=1)
//   3. set(key, "v2", cas=<got cas>) -> Ok   (CAS-matched update; cas -> 2)
//   4. set(key, "v3", cas=1)         -> Err  (stale cas; entry UNCHANGED, stays "v2")
//
// Each step's outcome is logged with a distinctive prefix so the harness has
// a secondary observable, but the PRIMARY assertion is host-side:
// RootVM.GetSharedData("conformance-shared-key") must return ("v2", cas=2).
use proxy_wasm::hostcalls;
use proxy_wasm::traits::*;
use proxy_wasm::types::*;

const KEY: &str = "conformance-shared-key";

proxy_wasm::main! {{
    proxy_wasm::set_log_level(LogLevel::Info);
    proxy_wasm::set_root_context(|_| -> Box<dyn RootContext> { Box::new(Root) });
}}

struct Root;
impl Context for Root {}
impl RootContext for Root {
    fn on_vm_start(&mut self, _: usize) -> bool {
        // 1. Unconditional first write.
        match hostcalls::set_shared_data(KEY, Some(b"v1"), Some(0)) {
            Ok(()) => log_line("set-v1-cas0 ok"),
            Err(e) => log_line(&format!("set-v1-cas0 err {:?}", e)),
        }

        // 2. Read back value + cas.
        let cas1 = match hostcalls::get_shared_data(KEY) {
            Ok((Some(v), cas)) => {
                log_line(&format!("get-1 value={} cas={:?}", String::from_utf8_lossy(&v), cas));
                cas
            }
            other => {
                log_line(&format!("get-1 unexpected {:?}", other));
                None
            }
        };

        // 3. CAS-matched update.
        match hostcalls::set_shared_data(KEY, Some(b"v2"), cas1) {
            Ok(()) => log_line("set-v2-casmatch ok"),
            Err(e) => log_line(&format!("set-v2-casmatch err {:?}", e)),
        }

        // 4. Stale-cas write — MUST be rejected; entry stays "v2".
        match hostcalls::set_shared_data(KEY, Some(b"v3"), Some(1)) {
            Ok(()) => log_line("set-v3-stalecas ok"),
            Err(e) => log_line(&format!("set-v3-stalecas err {:?}", e)),
        }

        true
    }
}

fn log_line(msg: &str) {
    let _ = hostcalls::log(LogLevel::Info, &format!("conformance-shared-data {}", msg));
}
