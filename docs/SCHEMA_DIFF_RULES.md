# Driftwood Schema & Contract Diffing Specifications

This document outlines the exact structural rules Driftwood uses to infer JSON schemas and evaluate breaking vs non-breaking contract changes.

## 1. Schema Inference Rules

Driftwood recursively parses JSON payloads into a schema tree of `JSONSchemaNode`:

| Primitive JSON Type | Inferred Driftwood Type | Notes |
| :--- | :--- | :--- |
| `"hello"` | `string` | UTF-8 String |
| `1024` | `integer` | Disambiguated from float using integer check |
| `98.6` | `number` | Floating point number |
| `true` / `false` | `boolean` | Boolean flag |
| `null` | `null` | Flagged as `Nullable: true` |
| `[...]` | `array` | Array item schemas are merged across elements |
| `{...}` | `object` | Key-value map of properties & required keys |

## 2. Structural Diff Rules & Severity

When comparing an observed JSON payload against a locked baseline:

| Condition | Detected Delta Kind | Severity | Description |
| :--- | :--- | :--- | :--- |
| Type mutated (e.g. `int` ➔ `string`) | `TYPE_MISMATCH` | 🚨 **BREAKING** | Frontend expecting integer will receive string. |
| Key present in baseline is missing | `REMOVED_FIELD` | 🚨 **BREAKING** | Component accessing property will receive `undefined`. |
| Expected non-null value is `null` | `NULLABILITY_CHANGE` | 🚨 **BREAKING** | Accessing nested properties on `null` causes runtime crashes. |
| New key present in payload | `ADDED_FIELD` | ℹ️ **INFO** | Backward-compatible extension. |
