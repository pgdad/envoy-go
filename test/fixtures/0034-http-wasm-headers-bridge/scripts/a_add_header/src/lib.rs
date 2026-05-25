// Scenario (a) add-fixed-header per phase 25.1 SPEC §9.1 row (a).
//
// Verbatim from SPEC + PLAN Task 15:
//   proxy_add_header_map_value(HTTP_REQUEST_HEADERS, "x-wasm-injected", "hello")
//
// Both the reference Envoy v1.34.0 (V8 runtime) and envoy-go (wazero
// runtime) load this module via the DataSource.Filename arm pointing at
// the vendored ../../bytecode/a_add_header.wasm blob; the per-stream
// proxy_on_request_headers callback fires `add_http_request_header`
// which lowers to `proxy_add_header_map_value(0, ...)` per the proxy-
// wasm-rust-sdk 0.2.4 mapping (type 0 = HttpRequestHeaders).
//
// The reflected request header `x-wasm-injected: hello` is asserted by
// the driver's classifyBody scenario (a) arm after the echobackend
// reflects the request headers as JSON in the response body.
//
// §4.5 guardrail compliance: only 24-hostcall surface; HTTP/1.1 only;
// no memory traps; no float-formatted logs.
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
        self.add_http_request_header("x-wasm-injected", "hello");
        Action::Continue
    }
}
