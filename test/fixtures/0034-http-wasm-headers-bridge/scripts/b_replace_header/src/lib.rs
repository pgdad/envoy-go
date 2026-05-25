// Scenario (b) replace-header per phase 25.1 SPEC §9.1 row (b).
//
// Verbatim from SPEC + PLAN Task 15:
//   proxy_replace_header_map_value(HTTP_REQUEST_HEADERS, "user-agent",
//                                  "envoy-go-wasm/1.0")
//
// The proxy-wasm-rust-sdk 0.2.4 maps `set_http_request_header(name,
// Some(value))` to `proxy_replace_header_map_value(0, ...)` (type 0 =
// HttpRequestHeaders). The driver supplies a baseline `user-agent:
// integration-test/0.1` so the replace has something to replace; the
// reflected request user-agent is asserted by classifyBody scenario (b).
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
        self.set_http_request_header("user-agent", Some("envoy-go-wasm/1.0"));
        Action::Continue
    }
}
