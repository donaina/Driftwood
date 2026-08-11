export interface APIDiffOptions {
  port?: number;
  target?: string;
}

export class APIDiff {
  constructor(options?: APIDiffOptions);
  start(): Promise<APIDiff>;
  stop(): void;
  middleware(): (req: any, res: any, next: () => void) => void;
}

export default APIDiff;
