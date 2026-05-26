// Scenario (m) httpCall-unknown-cluster per phase 25.2 SPEC §8.1.1
// row (m).
//
// SUBJECT-ONLY. Guest invokes proxy_http_call("nonexistent_cluster", ...)
// + records the dispatch_http_call return Status code as response
// header `x-httpcall-result: <code>`. Expected: BadArgument (2).
use proxy_wasm::traits::*;
use proxy_wasm::types::*;
use std::time::Duration;

proxy_wasm::main! {{
    proxy_wasm::set_log_level(LogLevel::Info);
    proxy_wasm::set_root_context(|_| -> Box<dyn RootContext> { Box::new(Root) });
}}

struct Root;
impl Context for Root {}
impl RootContext for Root {
    fn create_http_context(&self, _: u32) -> Option<Box<dyn HttpContext>> {
        Some(Box::new(Filter { code: 0 }))
    }
    fn get_type(&self) -> Option<ContextType> {
        Some(ContextType::HttpContext)
    }
}

struct Filter {
    code: u32,
}
impl Context for Filter {}
impl HttpContext for Filter {
    fn on_http_request_headers(&mut self, _: usize, _: bool) -> Action {
        match self.dispatch_http_call(
            "nonexistent_cluster",
            vec![(":method", "GET"), (":path", "/x"), (":authority", "nonexistent_cluster")],
            None,
            vec![],
            Duration::from_secs(5),
        ) {
            Ok(_) => self.code = 0,
            Err(s) => self.code = s as u32,
        }
        Action::Continue
    }
    fn on_http_response_headers(&mut self, _: usize, _: bool) -> Action {
        self.set_http_response_header("x-httpcall-result", Some(&self.code.to_string()));
        Action::Continue
    }
}
