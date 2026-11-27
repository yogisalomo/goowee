package core

// ProtocolVersion is the version of the mutation protocol shared by the WASM
// binary and runtime/goowee.js. Bump it when a mutation or the event reply
// format changes incompatibly: bridge.Run then refuses to mount against a
// runtime that speaks another version, instead of misapplying mutations.
const ProtocolVersion = 2 // 2: positional hydration (ADR-033)
