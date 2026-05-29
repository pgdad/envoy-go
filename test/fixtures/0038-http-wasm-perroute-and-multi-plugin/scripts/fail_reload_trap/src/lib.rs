// Fixture 0038 — fail_reload_trap guest per phase 25.3 Task 11.
//
// A FAIL_RELOAD plugin: when request header `x-trigger-trap: 1` is present
// the guest TRAPS (Rust panic → wasm unreachable → wazero RuntimeError).
// Under failure_policy=FAIL_RELOAD the envoy-go dispatch (decode_headers.go
// Task 9) catches the trap, returns 503 for the trapping request, and ARMS
// the reload state machine (NoteReloadRuntimeError). The NEXT request
// within the ~1s backoff window serves 503 + increments vm_reload_backoff;
// a request past the backoff window drives a reload attempt → reinstantiate
// the (valid) module → vm_reload_success + 200.
//
// Without the trigger header the guest adds a benign response header
// `x-reload: ok` and continues (200).
//
// This scenario is SUBJECT-ONLY: reference Envoy's V8 reload stat names
// differ from envoy-go's wasm.<plugin>.vm_reload_* triplet, so only the
// subject's counter progression is asserted (StatsAsserter).
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
        if let Some(v) = self.get_http_request_header("x-trigger-trap") {
            if v == "1" {
                // Trap: panic → wasm unreachable → host-side RuntimeError.
                panic!("fail_reload_trap: x-trigger-trap=1 triggered a guest trap");
            }
        }
        Action::Continue
    }
    fn on_http_response_headers(&mut self, _: usize, _: bool) -> Action {
        self.set_http_response_header("x-reload", Some("ok"));
        Action::Continue
    }
}
