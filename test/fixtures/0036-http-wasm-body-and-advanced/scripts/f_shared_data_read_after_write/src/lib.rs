// Scenario (f) shared-data-read-after-write per phase 25.2 SPEC §8.1.1
// row (f).
//
// On every request, the guest reads the shared-data key "counter"
// (default 0 on first read), increments it by 1 via CAS, and sets a
// response header `x-shared-data-counter: <value>`. The probe fires
// two requests sequentially against the listener; both sides should
// produce x-shared-data-counter=1 (first request) and =2 (second).
//
// Both V8 + wazero implement shared_data as a per-VM (per-plugin) K-V
// store keyed by config root_id; the CAS protocol is wire-identical.
use proxy_wasm::traits::*;
use proxy_wasm::types::*;

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
    fn on_http_response_headers(&mut self, _: usize, _: bool) -> Action {
        // Up to 3 CAS attempts to avoid races (not expected in this
        // single-stream test but defensive).
        for _ in 0..3 {
            let (data, cas) = self.get_shared_data("counter");
            let prev: u64 = match data.as_deref() {
                Some(b) => std::str::from_utf8(b)
                    .ok()
                    .and_then(|s| s.parse::<u64>().ok())
                    .unwrap_or(0),
                None => 0,
            };
            let next = prev + 1;
            let next_bytes = next.to_string().into_bytes();
            match self.set_shared_data("counter", Some(&next_bytes), cas) {
                Ok(()) => {
                    self.set_http_response_header("x-shared-data-counter", Some(&next.to_string()));
                    break;
                }
                Err(_) => continue,
            }
        }
        Action::Continue
    }
}
