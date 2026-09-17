import Anthropic from "@anthropic-ai/sdk";
import {
  ExplainRequest,
  ExplainResponse,
  ExplainError,
  StructuralDelta,
  DiffKind,
  DiffSeverity,
} from "./types";

const ANTHROPIC_API_KEY = process.env.ANTHROPIC_API_KEY;

/**
 * The model that writes the explanations.
 *
 * Overridable because the answer changes with time and this is not the place to
 * find out it did: it was pinned to claude-3-5-sonnet-20241022, and the same
 * literal was repeated in index.ts's /health response, so the two could
 * disagree about what was actually running. /health now reports this constant.
 */
export const MODEL = process.env.AI_MODEL || "claude-sonnet-5";

const client = ANTHROPIC_API_KEY
  ? new Anthropic({ apiKey: ANTHROPIC_API_KEY })
  : null;

const SYSTEM_PROMPT = `You are an expert API contract analyst. You receive structural diff deltas between a baseline API contract and an observed response. These deltas are purely structural (no prose, no emoji) and include:

- json_path: JSONPath to the affected field (bracket-quoted for special chars)
- kind: TYPE_MISMATCH | REMOVED_FIELD | ADDED_FIELD | NULLABILITY_CHANGE | ARRAY_ITEM_MISMATCH
- severity: BREAKING | WARNING | INFO
- expected: what the baseline contract expected
- actual: what was observed

Your task: provide a concise, actionable explanation for an API consumer.

Output ONLY valid JSON matching this schema:
{
  "summary": "One sentence: what changed",
  "impact": "Likely impact on API consumers (clients, integrations)",
  "root_cause": "Probable cause (e.g. backend schema change, migration, bug)",
  "suggested_action": "What the developer should do next",
  "confidence": 0.0-1.0
}`;

function formatDeltas(deltas: StructuralDelta[]): string {
  return deltas
    .map((d) => {
      const path = d.json_path;
      const kind = d.kind;
      const sev = d.severity;
      const exp = d.expected;
      const act = d.actual;
      return `  ${path}: ${kind} (${sev}) expected=${exp} actual=${act}`;
    })
    .join("\n");
}

export async function explainDiffs(
  req: ExplainRequest
): Promise<ExplainResponse | ExplainError> {
  if (!client) {
    return {
      error: "ANTHROPIC_API_KEY not configured",
      code: "NO_API_KEY",
      details: "Set ANTHROPIC_API_KEY environment variable to enable AI explanations",
    };
  }

  const userPrompt = `Endpoint: ${req.endpoint}

Deltas:
${formatDeltas(req.deltas)}

${req.baseline_sample ? `Baseline sample:\n${req.baseline_sample}\n` : ""}
${req.current_sample ? `Current sample:\n${req.current_sample}\n` : ""}`;

  try {
    const response = await client.messages.create({
      model: MODEL,
      max_tokens: 500,
      temperature: 0.1,
      system: SYSTEM_PROMPT,
      messages: [{ role: "user", content: userPrompt }],
    });

    const content = response.content[0];
    if (content.type !== "text") {
      throw new Error("Unexpected response type from Anthropic");
    }

    // Parse JSON from response
    const jsonMatch = content.text.match(/\{[\s\S]*\}/);
    if (!jsonMatch) {
      throw new Error("No JSON found in response");
    }

    const parsed: ExplainResponse = JSON.parse(jsonMatch[0]);

    // Validate required fields
    if (
      !parsed.summary ||
      !parsed.impact ||
      !parsed.root_cause ||
      !parsed.suggested_action ||
      typeof parsed.confidence !== "number"
    ) {
      throw new Error("Invalid response structure");
    }

    // Clamp confidence
    parsed.confidence = Math.max(0, Math.min(1, parsed.confidence));

    return parsed;
  } catch (err) {
    console.error("[AI Explainer] Error:", err);
    return {
      error: "Failed to generate explanation",
      code: "API_ERROR",
      details: err instanceof Error ? err.message : String(err),
    };
  }
}

export function hasApiKey(): boolean {
  return !!ANTHROPIC_API_KEY;
}