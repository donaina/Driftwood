export interface DriftwoodOptions {
  port?: number;
  target?: string;
}

export class Driftwood {
  constructor(options?: DriftwoodOptions);
  start(): Promise<Driftwood>;
  stop(): void;
}

export default Driftwood;
