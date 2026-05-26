// Scenario (h) property-stream-info per phase 25.2 SPEC §8.1.1 row (h).
//
// Guest reads request.method + request.path via proxy_get_property and
// sets response headers reflecting both values. The driver issues a
// fixed GET against a known path so both sides produce identical header
// values.
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
        Some(Box::new(Filter { method: String::new(), path: String::new() }))
    }
    fn get_type(&self) -> Option<ContextType> {
        Some(ContextType::HttpContext)
    }
}

struct Filter {
    method: String,
    path: String,
}
impl Context for Filter {}
impl HttpContext for Filter {
    fn on_http_request_headers(&mut self, _: usize, _: bool) -> Action {
        if let Some(b) = self.get_property(vec!["request", "method"]) {
            if let Ok(s) = std::str::from_utf8(&b) {
                self.method = s.to_string();
            }
        }
        if let Some(b) = self.get_property(vec!["request", "path"]) {
            if let Ok(s) = std::str::from_utf8(&b) {
                self.path = s.to_string();
            }
        }
        Action::Continue
    }
    fn on_http_response_headers(&mut self, _: usize, _: bool) -> Action {
        self.set_http_response_header("x-prop-method", Some(&self.method));
        self.set_http_response_header("x-prop-path", Some(&self.path));
        Action::Continue
    }
}
