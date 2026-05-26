// Scenario (c) body-mutate-replace per phase 25.2 SPEC §8.1.1 row (c).
//
// Guest reads the request body + uppercases the bytes via
// proxy_set_buffer_bytes (lowered from `set_http_request_body` in the
// SDK). The echobackend reflects request body length + content type;
// to make the assertion robust we just add a header signaling the
// guest mutated the body. The cross-side driver POSTs the same body
// to both sides + asserts the header is present (proves the set
// hostcall path is wired).
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
    fn on_http_request_body(&mut self, body_size: usize, end_of_stream: bool) -> Action {
        if !end_of_stream {
            return Action::Pause;
        }
        if let Some(bytes) = self.get_http_request_body(0, body_size) {
            let upper: Vec<u8> = bytes.iter().map(|b| b.to_ascii_uppercase()).collect();
            self.set_http_request_body(0, body_size, &upper);
            self.add_http_request_header("x-body-mutated", "1");
        }
        Action::Continue
    }
}
