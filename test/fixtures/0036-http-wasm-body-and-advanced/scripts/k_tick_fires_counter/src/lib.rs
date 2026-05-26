// Scenario (k) tick-fires-counter per phase 25.2 SPEC §8.1.1 row (k).
//
// SUBJECT-ONLY (non-deterministic timing). Guest sets a 50ms tick period
// at proxy_on_configure + proxy_on_tick increments a Counter
// `tick_count` defined via proxy_define_metric. After 250ms probe wait
// the StatsAsserter asserts subject's tick_count >= 5.
//
// No header mutation — the cross-side wire output is irrelevant; the
// StatsAsserter is the only equivalence signal for this arm.
use proxy_wasm::hostcalls;
use proxy_wasm::traits::*;
use proxy_wasm::types::*;
use std::time::Duration;

proxy_wasm::main! {{
    proxy_wasm::set_log_level(LogLevel::Info);
    proxy_wasm::set_root_context(|_| -> Box<dyn RootContext> { Box::new(Root { metric_id: None }) });
}}

struct Root {
    metric_id: Option<u32>,
}
impl Context for Root {}
impl RootContext for Root {
    fn on_configure(&mut self, _: usize) -> bool {
        self.set_tick_period(Duration::from_millis(50));
        match hostcalls::define_metric(MetricType::Counter, "tick_count") {
            Ok(id) => self.metric_id = Some(id),
            Err(_) => {}
        }
        true
    }
    fn on_tick(&mut self) {
        if let Some(id) = self.metric_id {
            let _ = hostcalls::increment_metric(id, 1);
        }
    }
    fn create_http_context(&self, _: u32) -> Option<Box<dyn HttpContext>> {
        Some(Box::new(Filter))
    }
    fn get_type(&self) -> Option<ContextType> {
        Some(ContextType::HttpContext)
    }
}

struct Filter;
impl Context for Filter {}
impl HttpContext for Filter {}
