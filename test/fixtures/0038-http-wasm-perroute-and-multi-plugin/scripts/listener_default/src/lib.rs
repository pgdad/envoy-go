// Fixture 0038 — listener_default guest per phase 25.3 Task 11.
//
// Adds a distinctive response header `x-wasm-variant: listener`. This is
// the LISTENER-default plugin (the http_filters chain entry, no per-route
// override). The driver asserts:
//   - perroute_override route: the OVERRIDE's `override` value wins.
//   - perroute_listener_default route: this `listener` value applies
//     (no per-route TPFC → listener default plugin runs).
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
        self.set_http_response_header("x-wasm-variant", Some("listener"));
        Action::Continue
    }
}
