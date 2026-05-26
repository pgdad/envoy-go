// Scenario (b) body-mutate-passthrough per phase 25.2 SPEC §8.1.1 row (b).
//
// Guest reads the request body + computes a tag from its length parity
// (deterministic; same input → same output on both V8 and wazero) and
// sets a request header `x-body-tag: <even|odd>`. Body passes through
// upstream unchanged; the echobackend reflects request headers as JSON
// + the driver classifies the reflected header.
//
// Cross-side determinism: parity is a pure function of body length; no
// runtime-divergent state (no hashing — wazero + V8 host crypto libs
// could in principle differ; parity avoids that).
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
        let tag = if body_size % 2 == 0 { "even" } else { "odd" };
        self.add_http_request_header("x-body-tag", tag);
        Action::Continue
    }
}
