export type NodeType = 'agent' | 'skill' | 'shared-ref' | 'mcp-server';
export type EdgeType = 'delegates-to' | 'uses-skill' | 'uses-shared-ref' | 'uses-mcp';

export interface GraphNode {
  id: string;
  type: NodeType;
  name: string;
  description: string;
  category?: string;
  model?: string;
  tools?: string[];
  mcpServers?: string[];
  trustedAgents?: string[];
  resources?: string[];
  sourcePath?: string;
  whenToUse?: string;
  excerpt?: string;
  userInvocable?: boolean;
}

export interface GraphEdge {
  id: string;
  source: string;
  target: string;
  type: EdgeType;
}

export interface FlowNode {
  id: string;
  type: 'start' | 'end' | 'process' | 'decision';
  label: string;
  agent?: string;
  description?: string;
  optional?: boolean;
}

export interface FlowEdge {
  from: string;
  to: string;
  label?: string;
  loop?: boolean;
}

export interface Workflow {
  id: string;
  name: string;
  description: string;
  orchestrator: string;
  nodes: FlowNode[];
  edges: FlowEdge[];
}

export interface EvalAgentSummary {
  agent: string;
  model: string;
  runs: number;
  avgIn: number;
  avgOut: number;
  avgDurationS: number;
  totalCostUsd: number;
}

export interface EvalStats {
  fullTestCount: number;
  smokeTestCount: number;
  byAgent: EvalAgentSummary[];
  grandRuns: number;
  grandTotalCostUsd: number;
}

export interface StatsData {
  counts: {
    agents: number;
    skills: number;
    sharedRefs: number;
    mcpServers: number;
    workflows: number;
  };
  modelDistribution: Record<string, number>;
  agentCategories: Record<string, number>;
  skillCategories: Record<string, number>;
  evals: EvalStats;
}

export interface GraphData {
  nodes: GraphNode[];
  edges: GraphEdge[];
  workflows: Workflow[];
  stats: StatsData;
}
