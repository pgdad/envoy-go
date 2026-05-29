// Conformance family `wasm_vm` — VM init + per-stream context isolation.
//
// Each HTTP stream gets its OWN Filter (HttpContext) instance with a private
// `calls` counter. On each request-headers call the guest increments its
// counter and writes the current value to the response header x-stream-count.
//
// The harness drives stream A twice (-> "1", "2") and stream B once (-> "1").
// Independent per-stream counters prove context isolation: if state were a
// shared global, B would report "3".
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
        Some(Box::new(Filter { calls: 0 }))
    }
    fn get_type(&self) -> Option<ContextType> {
        Some(ContextType::HttpContext)
    }
}

struct Filter {
    calls: u32,
}
impl Context for Filter {}
impl HttpContext for Filter {
    fn on_http_request_headers(&mut self, _: usize, _: bool) -> Action {
        self.calls += 1;
        self.set_http_response_header("x-stream-count", Some(&self.calls.to_string()));
        Action::Continue
    }
}
