// Fixture 0038 — shared_data_combined guest per phase 25.3 Task 11.
//
// ONE crate serving BOTH filters of the multi-plugin vm_id-shared chain.
// The SAME .wasm (identical code) is wired into two `envoy.filters.http.wasm`
// filters in ONE chain, BOTH with the SAME vm_id (+ same empty
// vm_configuration). Per the registry key
// Sha256(vm_id || vm_configuration || code) the two plugins SHARE one
// *RootVM (refcount) + ONE shared-data namespace (AMEND-C1 + Task 2). The
// two filters MUST use identical code for the VM-sharing key to match, so
// the guest carries NO per-filter role — it is fully symmetric.
//
// DISCRIMINATING shared-data proof (no plugin-configuration read required —
// envoy-go at 25.3 recognizes but does NOT route the PluginConfiguration
// buffer, so get_plugin_configuration is unavailable to the guest):
//
//   On the REQUEST path each filter does a CAS read-increment-write of the
//   shared-data counter key "x0038-count" (default 0). With TWO filters
//   sharing ONE namespace, filter A increments 0→1 and filter B increments
//   1→2 (the request path runs A before B). On the RESPONSE path each filter
//   reads the counter and sets `x-shared-count: <value>` (last writer wins;
//   the value is identical regardless of order). With sharing the final
//   reflected value is 2; WITHOUT sharing each filter has its OWN namespace
//   and increments its own 0→1, so the value would be 1. So
//   x-shared-count=2 ⇔ vm_id-scoped shared data is visible across the two
//   plugins on BOTH reference Envoy (cpp-host shares by vm_id) AND envoy-go.
use proxy_wasm::traits::*;
use proxy_wasm::types::*;

const KEY: &str = "x0038-count";

proxy_wasm::main! {{
    proxy_wasm::set_log_level(LogLevel::Info);
    proxy_wasm::set_root_context(|_| -> Box<dyn RootContext> { Box::new(Root) });
}}

struct Root;
impl Context for Root {}
impl RootContext for Root {
    fn create_http_context(&self, _: u32) -> Option<Box<dyn HttpContext>> {
        Some(Box::new(Filter))
    }
    fn get_type(&self) -> Option<ContextType> {
        Some(ContextType::HttpContext)
    }
}

struct Filter;
impl Context for Filter {}
impl HttpContext for Filter {
    fn on_http_request_headers(&mut self, _: usize, _: bool) -> Action {
        // CAS read-increment-write of the shared counter. Up to 5 attempts to
        // tolerate the (single-stream, but defensive) CAS race.
        for _ in 0..5 {
            let (data, cas) = self.get_shared_data(KEY);
            let prev: u64 = match data.as_deref() {
                Some(b) => std::str::from_utf8(b)
                    .ok()
                    .and_then(|s| s.parse::<u64>().ok())
                    .unwrap_or(0),
                None => 0,
            };
            let next = prev + 1;
            let next_bytes = next.to_string().into_bytes();
            match self.set_shared_data(KEY, Some(&next_bytes), cas) {
                Ok(()) => break,
                Err(_) => continue,
            }
        }
        Action::Continue
    }
    fn on_http_response_headers(&mut self, _: usize, _: bool) -> Action {
        let (data, _) = self.get_shared_data(KEY);
        let value = match data.as_deref() {
            Some(b) => String::from_utf8_lossy(b).to_string(),
            None => "0".to_string(),
        };
        self.set_http_response_header("x-shared-count", Some(&value));
        Action::Continue
    }
}
