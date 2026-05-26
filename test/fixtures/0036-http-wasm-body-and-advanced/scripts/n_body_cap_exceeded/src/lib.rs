// Scenario (n) body-cap-exceeded per phase 25.2 SPEC §8.1.1 row (n).
//
// SUBJECT-ONLY. Guest returns Action::Pause in proxy_on_request_body
// indefinitely so the host accumulates the body. The driver POSTs a
// body larger than envoy-go-strict body_buffer_cap_bytes (set to 1024
// in envoy-go.yaml) so the host responds 413 + bumps
// body_buffer_cap_exceeded + envoy_go.failures counters. The asserter
// scrapes both counters + asserts each >= 1.
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
    fn on_http_request_body(&mut self, _body_size: usize, _end_of_stream: bool) -> Action {
        // Pause indefinitely so the host accumulates body chunks +
        // enforces the body_buffer_cap_bytes ceiling.
        Action::Pause
    }
}
