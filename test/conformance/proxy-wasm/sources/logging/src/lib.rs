// Conformance family `logging` — emit a distinctive message at every
// proxy-wasm severity.
//
// On request headers the guest calls proxy_wasm::hostcalls::log directly
// (NOT the SDK `log!` macro, which filters client-side against the cached
// log level) at all five severities with messages the harness asserts on:
//
//   trace -> "conformance-logging trace-msg"
//   debug -> "conformance-logging debug-msg"
//   info  -> "conformance-logging info-msg"
//   warn  -> "conformance-logging warn-msg"
//   error -> "conformance-logging error-msg"
//
// The host bridge routes proxy_log -> ABICallbacks::Log; the conformance
// harness's recording callbacks forward (level, message) into the captured
// log buffer, so assertLogContains can verify each message + its level tag.
use proxy_wasm::traits::*;
use proxy_wasm::types::*;

proxy_wasm::main! {{
    proxy_wasm::set_log_level(LogLevel::Trace);
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
        let _ = proxy_wasm::hostcalls::log(LogLevel::Trace, "conformance-logging trace-msg");
        let _ = proxy_wasm::hostcalls::log(LogLevel::Debug, "conformance-logging debug-msg");
        let _ = proxy_wasm::hostcalls::log(LogLevel::Info, "conformance-logging info-msg");
        let _ = proxy_wasm::hostcalls::log(LogLevel::Warn, "conformance-logging warn-msg");
        let _ = proxy_wasm::hostcalls::log(LogLevel::Error, "conformance-logging error-msg");
        Action::Continue
    }
}
