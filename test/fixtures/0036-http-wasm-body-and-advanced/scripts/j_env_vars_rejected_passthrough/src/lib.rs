// Scenario (j) env-vars-rejected-passthrough per phase 25.2 SPEC §8.1.1
// row (j).
//
// Per AMEND-A6 25.2 does NOT yet activate env_vars (deferred to 25.3).
// std::env::vars() returns empty on both reference (V8) + subject
// (wazero) — count is precomputed at on_http_request_headers + stashed
// on the Filter struct so the response-header emit can be a single
// hostcall from on_http_response_headers without re-entering the
// proxy-wasm-rust-sdk 0.2.4 dispatcher's RefCell-guarded state mid-
// callback (the cross-callback hostcall pattern triggers a
// RefCell::borrow_mut panic in proxy_on_response_body on wazero per
// 25.2 IMPL Task 20 follow-up Concern 3 investigation).
//
// Per 25.2 IMPL Task 20 follow-up (Concern 3) — REWRITTEN to avoid the
// cross-callback hostcall re-entry pattern.
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
        Some(Box::new(Filter { env_count: 0 }))
    }
    fn get_type(&self) -> Option<ContextType> {
        Some(ContextType::HttpContext)
    }
}

struct Filter {
    // Precomputed at request_headers; emitted at response_headers without
    // re-invoking std::env or any other WASI surface during the response
    // callback. Avoids the SDK 0.2.4 RefCell re-entry panic per 25.2 IMPL
    // Task 20 follow-up Concern 3.
    env_count: usize,
}
impl Context for Filter {}
impl HttpContext for Filter {
    fn on_http_request_headers(&mut self, _: usize, _: bool) -> Action {
        // Compute the env-var count here (early in the lifecycle, while
        // the SDK dispatcher cell is in a clean state). Both reference
        // (V8 default) + subject (envoy-go-strict WASI environ_get
        // returns empty per AMEND-A6) yield 0.
        self.env_count = std::env::vars().count();
        Action::Continue
    }

    fn on_http_response_headers(&mut self, _: usize, _: bool) -> Action {
        // Single hostcall from this callback; the precomputed env_count
        // is emitted verbatim — no cross-callback re-entry.
        self.add_http_response_header("x-env-keys", &self.env_count.to_string());
        Action::Continue
    }
}
