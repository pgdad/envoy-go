// Scenario (i) metric-define-only per phase 25.2 SPEC §8.1.1 row (i).
//
// Guest defines a Counter via proxy_define_metric + increments it once
// in proxy_on_request_headers. The cross-side assertion ignores stats
// (the dynamic stat is subject-only) and only compares response bytes,
// which both sides produce identically (no header mutation).
use proxy_wasm::hostcalls;
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
        if let Ok(id) = hostcalls::define_metric(MetricType::Counter, "scenario_i_counter") {
            let _ = hostcalls::increment_metric(id, 1);
        }
        // Mark presence so the cross-side driver can verify the path
        // executed (best-effort; both runtimes route the hostcall).
        self.set_http_request_header("x-metric-defined", Some("1"));
        Action::Continue
    }
}
