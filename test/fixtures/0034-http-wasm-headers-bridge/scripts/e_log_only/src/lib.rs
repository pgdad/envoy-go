// Scenario (e) log-only-passthrough per phase 25.1 SPEC §9.1 row (e).
//
// Verbatim from SPEC + PLAN Task 15:
//   proxy_log(INFO, "wasm hit")
//
// No header mutation; request passes through to echobackend unchanged.
// The cross-side stat-counter delta IS the "wasm ran" assertion (the
// literal log line is NOT cross-side asserted — wazero log sink format
// vs reference Envoy spdlog format diverges per parent guidance,
// mirroring fixture-0026's D3 closure for lua).
//
// The StatsAsserter.AssertStats scrapes /stats/prometheus on both sides
// after Drive + asserts both `wasm.<plugin>.executions` counters
// increment by 1 per probe.
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
        // Direct proxy_log invocation per parent SPEC verbatim. The
        // higher-level `info!()` macro requires the `log` crate facade
        // which is not pulled by proxy-wasm-rust-sdk 0.2.4 by default;
        // calling hostcalls::log directly keeps the dependency surface
        // pinned to proxy-wasm =0.2.4 only.
        let _ = hostcalls::log(LogLevel::Info, "wasm hit");
        Action::Continue
    }
}
