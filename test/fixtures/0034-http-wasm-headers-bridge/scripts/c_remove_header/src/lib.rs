// Scenario (c) remove-header per phase 25.1 SPEC §9.1 row (c).
//
// Verbatim from SPEC + PLAN Task 15:
//   proxy_remove_header_map_value(HTTP_REQUEST_HEADERS, "x-blocked")
//
// The proxy-wasm-rust-sdk 0.2.4 maps `set_http_request_header(name,
// None)` to `proxy_remove_header_map_value(0, ...)` (type 0 =
// HttpRequestHeaders). The driver supplies `x-blocked: yes` in the
// request so the remove has something to remove; classifyBody scenario
// (c) asserts the absence of `x-blocked` in the echobackend-reflected
// request headers JSON.
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
        self.set_http_request_header("x-blocked", None);
        Action::Continue
    }
}
