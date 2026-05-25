// Scenario (d) respond-shortcircuit per phase 25.1 SPEC §9.1 row (d).
//
// Verbatim from SPEC + PLAN Task 15:
//   proxy_send_local_response(403, "Forbidden", "denied", &[], 0)
//
// The proxy-wasm-rust-sdk 0.2.4 maps `send_http_response(status,
// headers, body)` to `proxy_send_local_response(...)` with the
// arguments above. The guest must then return Action::Pause so the
// host short-circuits the upstream round-trip + replies to the
// downstream with the captured local-response payload.
//
// Cross-side byte-pin (driver classifyBody scenario (d)):
//   - status 403
//   - body "denied" (6 bytes, no trailing newline)
//   - content-length: 6 (auto-set by host)
//   - content-type: text/plain (host default)
//   - NO upstream round-trip (echobackend never sees the request)
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
    fn on_http_request_headers(&mut self, _: usize, _: bool) -> Action {
        self.send_http_response(403, vec![], Some(b"denied"));
        Action::Pause
    }
}
