// Fixture 0038 — perroute_override guest per phase 25.3 Task 11.
//
// Adds a distinctive response header `x-wasm-variant: override`. Used as
// the per-route wholesale Wasm TPFC override (AMEND-C1: the ENTIRE Wasm
// message — VM included — swaps when a route carries a per-route
// typed_per_filter_config). The driver asserts the OVERRIDE's header
// value `override` (NOT the listener default's `listener`) is present on
// the response, proving the per-route override took over the stream on
// BOTH reference Envoy (cpp-host wholesale per-route Wasm) AND envoy-go.
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
        self.set_http_response_header("x-wasm-variant", Some("override"));
        Action::Continue
    }
}
