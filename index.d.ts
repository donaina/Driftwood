export interface DriftwoodOptions {
  port?: number;
  target?: string;
}

export class Driftwood {
  constructor(options?: DriftwoodOptions);
  start(): Promise<Driftwood>;
  stop(): void;
  middleware(): (req: any, res: any, next: () => void) => void;
}

export default Driftwood;
