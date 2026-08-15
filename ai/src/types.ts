/**
 * Input/output types for AI diff explanation service
 * These match the structural deltas from the Go diff engine (PR #2)
 */

export interface StructuralDelta {
  json_path: string;
  kind: DiffKind;
  severity: DiffSeverity;
  expected: string;
  actual: string;
}

export enum DiffKind {
  TYPE_MISMATCH = "TYPE_MISMATCH",
  REMOVED_FIELD = "REMOVED_FIELD",
  ADDED_FIELD = "ADDED_FIELD",
  NULLABILITY_CHANGE = "NULLABILITY_CHANGE",
  ARRAY_ITEM_MISMATCH = "ARRAY_ITEM_MISMATCH",
}

export enum DiffSeverity {
  BREAKING = "BREAKING",
  WARNING = "WARNING",
  INFO = "INFO",
}

export interface ExplainRequest {
  endpoint: string;           // e.g. "GET /api/users"
  deltas: StructuralDelta[];  // structural deltas (no prose/emoji)
  baseline_sample?: string;   // optional: original baseline payload
  current_sample?: string;    // optional: current payload that triggered diff
}

export interface ExplainResponse {
  summary: string;            // one-sentence summary
  impact: string;             // likely impact on consumers
  root_cause: string;         // probable cause of the drift
  suggested_action: string;   // what the developer should do
  confidence: number;         // 0-1 confidence in explanation
}

export interface ExplainError {
  error: string;
  code: "NO_API_KEY" | "API_ERROR" | "INVALID_INPUT" | "TIMEOUT";
  details?: string;
}

// Health check response
export interface HealthResponse {
  status: "healthy" | "degraded";
  has_api_key: boolean;
  model: string;
}