// Scenario (g) foreign-function-deny-default per phase 25.2 SPEC §8.1.1
// row (g).
//
// Guest invokes proxy_call_foreign_function("verify_signature") + sets a
// response header `x-foreign-result: <wasmresult>`. Per AMEND-A9 the
// envoy-go ForeignFunctionRegistry is EMPTY at default config — every
// call returns WasmResult::NotFound (= 1). Reference Envoy v1.37.2 V8
// runtime: foreign functions can be registered via std::function
// registry; for an unknown name the call also returns NotFound.
//
// Cross-side determinism: both sides return NotFound (1) for an unknown
// foreign function name.
use proxy_wasm::traits::*;
use proxy_wasm::types::*;
use proxy_wasm::hostcalls;

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
    fn on_http_response_headers(&mut self, _: usize, _: bool) -> Action {
        let result = hostcalls::call_foreign_function("verify_signature", None);
        // Both Ok(_) and Err(Status) carry a numeric code; we encode the
        // status numerically. Status::NotFound = 1 per proxy-wasm-rust-sdk.
        let code: u32 = match result {
            Ok(_) => 0, // Ok = 0
            Err(s) => s as u32,
        };
        self.set_http_response_header("x-foreign-result", Some(&code.to_string()));
        Action::Continue
    }
}
