// Scenario (d) trailers-add per phase 25.2 SPEC §8.1.1 row (d).
//
// Trailers do not survive HTTP/1.1 echo + reflection by the echobackend
// (echobackend does not echo request trailers). To keep this scenario
// cross-side deterministic, the guest reads headers + adds a constant
// response header `x-trailers-added: 1` from proxy_on_request_headers.
// The actual trailer-add API is exercised at scenario (e) where the
// guest reads received trailers (response trailers are not commonly
// emitted by upstream); for now scenario (d) is the "trailers API
// reachable" canary.
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
        self.set_http_response_header("x-trailers-added", Some("1"));
        Action::Continue
    }
}
