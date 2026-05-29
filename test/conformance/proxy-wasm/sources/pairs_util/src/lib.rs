// Conformance family `pairs_util` — pairs wire-format round-trip.
//
// On request headers the guest reads the FULL request-header map via
// get_map(HttpRequestHeaders) — the proxy_get_header_map_pairs hostcall whose
// host side serializes the map with the proxy-wasm pairs wire format
// (count-prefixed length-delimited key/value pairs; envoy-go EncodePairs).
// The SDK decodes that buffer back into Vec<(String,String)>.
//
// The guest then writes two response headers reflecting what it decoded:
//   x-pairs-count: <decoded pair count>
//   x-pairs-echo:  <value of the seeded "x-probe" key, or "MISSING">
//
// The harness seeds the request-header map + asserts these reflect the seed,
// proving the pairs marshalling is byte-faithful end to end.
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
        let count = pairs.len();
        let echo = pairs
            .iter()
            .find(|(k, _)| k == "x-probe")
            .map(|(_, v)| v.clone())
            .unwrap_or_else(|| "MISSING".to_string());
        self.set_http_response_header("x-pairs-count", Some(&count.to_string()));
        self.set_http_response_header("x-pairs-echo", Some(&echo));
        Action::Continue
    }
}
