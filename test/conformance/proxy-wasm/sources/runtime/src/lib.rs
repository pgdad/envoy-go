// Conformance family `runtime` — guest trap surfacing.
//
// On request headers the guest deliberately traps: `unreachable!()` panics,
// and with panic="abort" the panic lowers to the wasm `unreachable` opcode,
// trapping the instance. The harness asserts CallProxyOnRequestHeaders returns
// a NON-NIL error (the host runtime catches the wazero trap and surfaces it).
//
// The harness runs this family on its OWN fresh RootVM because a trap poisons
// the proxy-wasm instance (the known RefCell-borrow semantics) — keeping it
// self-contained prevents the poisoned instance from leaking into other
// families that share the registry / process.
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
        // Deliberate trap: lowers to the wasm `unreachable` opcode.
        unreachable!("conformance-runtime deliberate trap");
    }
}
