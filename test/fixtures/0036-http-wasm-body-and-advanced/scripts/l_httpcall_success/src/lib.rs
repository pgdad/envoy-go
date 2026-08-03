// Scenario (l) httpCall-success per phase 25.2 SPEC §8.1.1 row (l).
//
// SUBJECT-ONLY. Guest invokes proxy_http_call("cluster_b", ...) at
// proxy_on_request_headers + sets response header `x-httpcall-status:
// <status>` from proxy_on_http_call_response. The StatsAsserter
// asserts http_call_dispatched + http_call_response increment.
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
        Some(Box::new(Filter { call_status: 0 }))
    }
    fn get_type(&self) -> Option<ContextType> {
        Some(ContextType::HttpContext)
    }
}

struct Filter {
    call_status: u32,
}
impl Context for Filter {
    fn on_http_call_response(&mut self, _: u32, _: usize, _: usize, _: usize) {
        let headers = self.get_http_call_response_headers();
        for (k, v) in headers.iter() {
            if k == ":status" {
                if let Ok(n) = v.parse::<u32>() {
                    self.call_status = n;
                }
            }
        }
        // Resume the request the on_http_request_headers Action::Pause halted.
        // Without this the stream never continues: reference Envoy holds the
        // request open until the downstream client times out.
        self.resume_http_request();
    }
}
impl HttpContext for Filter {
    fn on_http_request_headers(&mut self, _: usize, _: bool) -> Action {
        let _ = self.dispatch_http_call(
            "cluster_b",
            vec![
                (":method", "GET"),
                (":path", "/upstream_probe"),
                (":authority", "cluster_b"),
            ],
            None,
            vec![],
            Duration::from_secs(5),
        );
        Action::Pause
    }
    fn on_http_response_headers(&mut self, _: usize, _: bool) -> Action {
        self.set_http_response_header("x-httpcall-status", Some(&self.call_status.to_string()));
        Action::Continue
    }
}
