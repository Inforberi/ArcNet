export type Candidate = {host: string; sni: string};
export type RecommendationState = 'checking' | 'ready' | 'failed' | 'unavailable';
export type TargetResult = {candidate: Candidate; eligible: boolean; successes: number; median_ms: number; p95_ms: number; error?: string; checked_at: number; http_verified: boolean; vpn_verified: boolean};
export type TargetReport = {results: TargetResult[]; recommended: Candidate | null; measured_from: string; checked_at: number; expires_at: number; vpn_verified: boolean};
export type TargetJob = {version: number; id: string; node_id: string; state: string; expires_at: number; report?: TargetReport};
export const candidateKey = (candidate: Candidate) => `${candidate.host}/${candidate.sni}`;
export const usableResult = (job: TargetJob | undefined, nodeID: string, candidate: Candidate, now = Date.now() / 1000): TargetResult | undefined => {
  if (!job || job.version !== 1 || job.state !== 'done' || job.node_id !== nodeID || !job.report || job.report.measured_from !== nodeID || job.report.expires_at <= now) return;
  return job.report.results.find(result => candidateKey(result.candidate) === candidateKey(candidate) && result.eligible && result.successes === 3 && result.http_verified);
};
