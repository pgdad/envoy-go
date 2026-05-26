// Scenario (a) body-read-only per phase 25.2 SPEC §8.1.1 row (a).
//
// Guest reads the request body via proxy_get_buffer_bytes (lowered from
// `get_http_request_body` in the SDK) inside proxy_on_request_body, then
// adds a request header `x-body-len: <len>` reflecting the bytes seen
// before forwarding upstream. The echobackend reflects request headers
// as JSON; the driver classifies the reflected JSON for the header.
//
// Cross-side determinism: both V8 + wazero implement get_http_request_body
// identically per the proxy-wasm v0.2.1 spec; the accumulator size is
// the request body the driver POSTs (a fixed string).
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
        Some(Box::new(Filter { len_seen: 0 }))
    }
    fn get_type(&self) -> Option<ContextType> {
        Some(ContextType::HttpContext)
    }
}

struct Filter {
    len_seen: usize,
}
impl Context for Filter {}
impl HttpContext for Filter {
    fn on_http_request_body(&mut self, body_size: usize, end_of_stream: bool) -> Action {
        // Read what's accumulated so far. body_size is the accumulated total
        // per Q1; we read the full slice + record len_seen.
        if let Some(bytes) = self.get_http_request_body(0, body_size) {
            self.len_seen = bytes.len();
        }
        if !end_of_stream {
            // Wait for more chunks; the upstream call is held until
            // end-of-stream so the header is added on the final chunk.
            return Action::Pause;
        }
        self.add_http_request_header("x-body-len", &self.len_seen.to_string());
        Action::Continue
    }
}
