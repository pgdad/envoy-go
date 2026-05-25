// Scenario (f) header-iteration-count per phase 25.1 SPEC §9.1 row (f).
//
// Verbatim from SPEC + PLAN Task 15:
//   proxy_get_header_map_pairs(HTTP_REQUEST_HEADERS) → count pairs →
//   add "x-headers-count: N"
//
// The proxy-wasm-rust-sdk 0.2.4 maps `get_http_request_headers()` to
// `proxy_get_header_map_pairs(0)` (type 0 = HttpRequestHeaders) which
// returns the full pair list. The guest counts entries + adds
// `x-headers-count: <N>` header which the driver asserts via the
// echobackend-reflected JSON.
//
// Cross-side determinism note: parent §4.5 D6 guardrail (b) pins the
// pair-emission order to sorted-by-name on BOTH runtimes via the host-
// side `GetHeaderMap` impl (envoy-go) / native sorted iteration
// (upstream); the COUNT N is order-independent so cross-side byte-exact
// holds regardless of iteration order.
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
        let pairs = self.get_http_request_headers();
        let n = pairs.len();
        self.add_http_request_header("x-headers-count", &n.to_string());
        Action::Continue
    }
}
