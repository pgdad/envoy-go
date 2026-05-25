// Scenario (g) property-read-method per phase 25.1 SPEC §9.1 row (g).
//
// Verbatim from SPEC + PLAN Task 15:
//   proxy_get_property("request.method") → add "x-request-method: GET"
//
// The proxy-wasm-rust-sdk 0.2.4 maps `get_property(&["request",
// "method"])` to `proxy_get_property(...)` with the dotted-path encoded
// as a null-byte-separated byte sequence. The envoy-go GetProperty
// hostcall (abi_callbacks.go getRequestProperty) returns the
// :method pseudo-header bytes; reference Envoy's V8 host implements
// the standard CEL `request.method` property accessor with the same
// semantics.
//
// Cross-side byte-pin: the driver always issues GET so both sides
// produce `x-request-method: GET` in the reflected request.
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
        if let Some(bytes) = self.get_property(vec!["request", "method"]) {
            if let Ok(method) = std::str::from_utf8(&bytes) {
                self.add_http_request_header("x-request-method", method);
            }
        }
        Action::Continue
    }
}
