// Conformance family `security` — per-capability hostcall gating.
//
// On request headers the guest issues TWO hostcalls in order:
//
//   1. proxy_log("conformance-security log-ok")  — always-allowed; the log
//      line landing proves proxy_log is permitted.
//   2. proxy_get_current_time()                  — gated. The proxy-wasm SDK
//      PANICS ("unexpected status") on any non-Ok host result (it has no
//      graceful Err path for the deny sentinel), so a DENIED capability lowers
//      to a wasm trap. On success the guest logs "conformance-security time-ok".
//
// The harness drives the SAME guest under two sandboxes:
//   - permissive: time hostcall ALLOWED -> no trap; log shows "log-ok"+"time-ok".
//   - restricted: proxy_get_current_time_nanoseconds DENIED -> the host returns
//     the deny sentinel, the SDK traps; CallProxyOnRequestHeaders returns a
//     non-nil error. "log-ok" still landed (proxy_log allowed, ran first) but
//     "time-ok" did NOT — proving the gate is per-capability, not all-or-nothing.
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
        // Always-allowed hostcall: its log line landing proves proxy_log works.
        // Emitted FIRST so it lands even when the gated call below traps.
        let _ = hostcalls::log(LogLevel::Info, "conformance-security log-ok");

        // Gated hostcall. The SDK panics (-> wasm trap) on the deny sentinel,
        // so when proxy_get_current_time_nanoseconds is DENIED this call traps
        // before reaching the log below. When ALLOWED it returns Ok and we log.
        let _ = hostcalls::get_current_time();
        let _ = hostcalls::log(LogLevel::Info, "conformance-security time-ok");

        Action::Continue
    }
}
