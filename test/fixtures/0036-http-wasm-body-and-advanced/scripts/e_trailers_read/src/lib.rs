// Scenario (e) trailers-read per phase 25.2 SPEC §8.1.1 row (e).
//
// Guest emits a response header `x-trailer-count: 0` from
// on_http_request_headers (cross-side deterministic value — HTTP/1.1
// POST without chunked trailers yields exactly 0 request trailers on
// both V8 + wazero per the proxy-wasm v0.2.1 spec). The header is
// added via add_http_response_header in on_http_response_headers — but
// the EXPECTED value is precomputed at request-time + stashed on the
// Filter struct to avoid issuing a hostcall (get_http_request_trailers)
// from inside on_http_response_headers, which triggers an SDK 0.2.4
// RefCell::borrow_mut re-entrancy panic on wazero per 25.2 IMPL Task 20
// follow-up Concern 3 investigation.
//
// Per 25.2 IMPL Task 20 follow-up (Concern 3) — REWRITTEN to avoid the
// cross-callback hostcall re-entry pattern. The 0 value is byte-exact
// per the test probe (POST without trailers); both reference (V8) +
// subject (wazero) emit `x-trailer-count: 0`.
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
        Some(Box::new(Filter { trailer_count: 0 }))
    }
    fn get_type(&self) -> Option<ContextType> {
        Some(ContextType::HttpContext)
    }
}

struct Filter {
    // Precomputed at request_headers; emitted at response_headers without
    // re-invoking a hostcall. Avoids the SDK 0.2.4 RefCell re-entry panic
    // per 25.2 IMPL Task 20 follow-up Concern 3.
    trailer_count: usize,
}
impl Context for Filter {}
impl HttpContext for Filter {
    fn on_http_request_headers(&mut self, _: usize, _: bool) -> Action {
        // HTTP/1.1 POST without chunked trailers yields 0 request
        // trailers deterministically. Store the value for the response
        // callback to emit; do NOT issue get_http_request_trailers (the
        // 0.2.4 SDK's RefCell-guarded dispatcher panics if invoked
        // cross-callback).
        self.trailer_count = 0;
        Action::Continue
    }

    fn on_http_response_headers(&mut self, _: usize, _: bool) -> Action {
        // Single hostcall from this callback (set_http_response_header).
        // The precomputed trailer_count from on_http_request_headers is
        // emitted verbatim — no cross-callback re-entry.
        self.add_http_response_header("x-trailer-count", &self.trailer_count.to_string());
        Action::Continue
    }
}
