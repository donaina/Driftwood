import express, { Request, Response, NextFunction } from "express";
import { explainDiffs, hasApiKey } from "./explainer";
import {
  ExplainRequest,
  ExplainResponse,
  ExplainError,
  HealthResponse,
} from "./types";

const app = express();
const PORT = process.env.AI_PORT || process.env.PORT || 8788;

app.use(express.json({ limit: "1mb" }));

// Request logging
app.use((req: Request, _res: Response, next: NextFunction) => {
  console.log(`[AI] ${req.method} ${req.path}`);
  next();
});

// Health check
app.get("/health", (_req: Request, res: Response<HealthResponse>) => {
  res.json({
    status: hasApiKey() ? "healthy" : "degraded",
    has_api_key: hasApiKey(),
    model: "claude-3-5-sonnet-20241022",
  });
});

// Main explanation endpoint
app.post(
  "/explain",
  async (req: Request<{}, ExplainResponse | ExplainError, ExplainRequest>, res: Response<ExplainResponse | ExplainError>) => {
    try {
      const { endpoint, deltas, baseline_sample, current_sample } = req.body;

      // Validate required fields
      if (!endpoint || typeof endpoint !== "string") {
        return res.status(400).json({
          error: "Missing or invalid 'endpoint'",
          code: "INVALID_INPUT",
        });
      }

      if (!Array.isArray(deltas) || deltas.length === 0) {
        return res.status(400).json({
          error: "Missing or empty 'deltas' array",
          code: "INVALID_INPUT",
        });
      }

      // Validate delta structure
      for (const d of deltas) {
        if (
          !d.json_path ||
          !d.kind ||
          !d.severity ||
          !d.expected ||
          !d.actual
        ) {
          return res.status(400).json({
            error: "Invalid delta structure",
            code: "INVALID_INPUT",
          });
        }
      }

      const result = await explainDiffs({
        endpoint,
        deltas,
        baseline_sample,
        current_sample,
      });

      if ("error" in result) {
        const status = result.code === "NO_API_KEY" ? 503 : 500;
        return res.status(status).json(result);
      }

      res.json(result);
    } catch (err) {
      console.error("[AI] Unexpected error:", err);
      res.status(500).json({
        error: "Internal server error",
        code: "API_ERROR",
        details: err instanceof Error ? err.message : String(err),
      });
    }
  }
);

// Start server
app.listen(PORT, () => {
  console.log(`[AI] Driftwood AI Explainer listening on http://localhost:${PORT}`);
  console.log(`[AI] API Key: ${hasApiKey() ? "CONFIGURED" : "NOT SET (degraded mode)"}`);
});

export { app };