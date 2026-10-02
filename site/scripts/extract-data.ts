import { readFileSync, readdirSync, writeFileSync, existsSync } from 'fs';
import { join, basename, resolve } from 'path';
import { parse as parseYaml } from 'yaml';
import TOML from '@iarna/toml';

const ROOT = resolve(
  process.env.CATALOG_ROOT || resolve(import.meta.dirname, '..', '..'),
);
const AGENTS_DIR = join(ROOT, 'agents');
const SKILLS_DIR = join(ROOT, 'skills');
const SHARED_DIR = join(SKILLS_DIR, '_shared');
const SKILL_PLATFORMS = ['universal', 'claude', 'codex'] as const;
const OUTPUT = resolve(
  process.env.CATALOG_OUTPUT || join(import.meta.dirname, '..', 'src', 'data', 'graph.json'),
);
const LITMUS_RESULTS_DIR = join(ROOT, 'litmus', 'results');

// Category mappings from README
const AGENT_CATEGORIES: Record<string, string> = {
  'photon-lead': 'Orchestration',
  'builder': 'Implementation',
  'tester': 'Quality Assurance',
  'validator': 'Quality Assurance',
  'code-reviewer': 'Quality Assurance',
  'researcher': 'Research',
  'research-validator': 'Research',
  'explore': 'Research',
};

const SKILL_CATEGORIES: Record<string, string> = {
  'review-code': 'Code Review',
  'review-pr': 'Code Review',
  'code-review-excellence': 'Code Review',
  'review-addressed-comments': 'Code Review',
  'write-pr': 'Writing & Documentation',
  'write-pr-comments': 'Writing & Documentation',
  'pr-comment-humanizer': 'Writing & Documentation',
  'implement-task': 'Development Workflows',
  'update-skill': 'Meta',
};

interface GraphNode {
  id: string;
  type: 'agent' | 'skill' | 'shared-ref' | 'mcp-server';
  name: string;
  description: string;
  category?: string;
  platforms?: ('claude' | 'codex' | 'kiro')[];
  compatibility?: 'universal' | 'claude' | 'codex' | 'variants';
  profile?: 'haiku' | 'sonnet' | 'opus';
  models?: Partial<Record<'claude' | 'codex' | 'kiro', { model: string; effort: string }>>;
  sources?: Partial<Record<'claude' | 'codex' | 'kiro' | 'universal', string>>;
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

interface GraphEdge {
  id: string;
  source: string;
  target: string;
  type: 'delegates-to' | 'uses-skill' | 'uses-shared-ref' | 'uses-mcp';
}

interface FlowNode {
  id: string;
  type: 'start' | 'end' | 'process' | 'decision';
  label: string;
  agent?: string;
  description?: string;
  optional?: boolean;
}

interface FlowEdge {
  from: string;
  to: string;
  label?: string;
  loop?: boolean;
}

interface Workflow {
  id: string;
  name: string;
  description: string;
  orchestrator: string;
  nodes: FlowNode[];
  edges: FlowEdge[];
}

interface EvalAgentSummary {
  agent: string;
  model: string;
  runs: number;
  avgIn: number;
  avgOut: number;
  avgDurationS: number;
  totalCostUsd: number;
}

interface EvalStats {
  caseCount: number;
  runCount: number;
  byAgent: EvalAgentSummary[];
  grandRuns: number;
  grandTotalCostUsd: number;
}

interface StatsData {
  counts: {
    agents: number;
    skills: number;
    sharedRefs: number;
    mcpServers: number;
    workflows: number;
  };
  profileDistribution: Record<string, number>;
  modelDistribution: Record<string, Record<string, number>>;
  agentCategories: Record<string, number>;
  skillCategories: Record<string, number>;
  evals: EvalStats;
}

interface GraphData {
  nodes: GraphNode[];
  edges: GraphEdge[];
  workflows: Workflow[];
  stats: StatsData;
}

function parseFrontmatter(content: string): Record<string, unknown> {
  const match = content.match(/^---\r?\n([\s\S]*?)\r?\n---/);
  if (!match) return {};
  const parsed = parseYaml(match[1]);
  return parsed && typeof parsed === 'object' && !Array.isArray(parsed)
    ? parsed as Record<string, unknown>
    : {};
}

function extractAgents(): { nodes: GraphNode[]; edges: GraphEdge[] } {
  const nodes: GraphNode[] = [];
  const edges: GraphEdge[] = [];

  const dirs = readdirSync(AGENTS_DIR, { withFileTypes: true })
    .filter(entry => entry.isDirectory())
    .map(entry => entry.name)
    .sort();

  for (const directoryName of dirs) {
    const directory = join(AGENTS_DIR, directoryName);
    const manifestPath = join(directory, 'manifest.json');
    if (!existsSync(manifestPath)) continue;

    let manifest: Record<string, unknown>;
    try {
      manifest = JSON.parse(readFileSync(manifestPath, 'utf-8')) as Record<string, unknown>;
    } catch {
      console.warn(`Skipping invalid manifest: ${directoryName}`);
      continue;
    }

    const name = String(manifest.name || directoryName);
    const id = `agent:${name}`;
    const sources: GraphNode['sources'] = {
      claude: `agents/${directoryName}/claude.md`,
      codex: `agents/${directoryName}/codex.toml`,
      kiro: `agents/${directoryName}/kiro.json`,
    };
    const models: NonNullable<GraphNode['models']> = {};
    let claudeFrontmatter: Record<string, unknown> = {};
    let codexConfig: Record<string, unknown> = {};
    let kiroConfig: Record<string, unknown> = {};

    const claudePath = join(directory, 'claude.md');
    if (existsSync(claudePath)) {
      claudeFrontmatter = parseFrontmatter(readFileSync(claudePath, 'utf-8'));
      if (typeof claudeFrontmatter.model === 'string') {
        models.claude = {
          model: claudeFrontmatter.model,
          effort: String(claudeFrontmatter.effort || 'unspecified'),
        };
      }
    }

    const codexPath = join(directory, 'codex.toml');
    if (existsSync(codexPath)) {
      try {
        codexConfig = TOML.parse(readFileSync(codexPath, 'utf-8')) as Record<string, unknown>;
      } catch {
        console.warn(`Skipping invalid Codex config: ${directoryName}`);
      }
      if (typeof codexConfig.model === 'string') {
        models.codex = {
          model: codexConfig.model,
          effort: String(codexConfig.model_reasoning_effort || 'unspecified'),
        };
      }
    }

    const kiroPath = join(directory, 'kiro.json');
    if (existsSync(kiroPath)) {
      try {
        kiroConfig = JSON.parse(readFileSync(kiroPath, 'utf-8')) as Record<string, unknown>;
      } catch {
        console.warn(`Skipping invalid Kiro config: ${directoryName}`);
      }
      if (typeof kiroConfig.model === 'string') {
        models.kiro = { model: kiroConfig.model, effort: 'unspecified' };
      }
    }

    const platforms = Array.isArray(manifest.platforms)
      ? manifest.platforms.filter(
        (platform): platform is 'claude' | 'codex' | 'kiro' =>
          platform === 'claude' || platform === 'codex' || platform === 'kiro',
      )
      : (Object.keys(models) as ('claude' | 'codex' | 'kiro')[]);

    // Extract trustedAgents from toolsSettings
    const toolsSettings = kiroConfig.toolsSettings as Record<string, Record<string, unknown>> | undefined;
    const trustedAgents = toolsSettings?.subagent?.trustedAgents as string[] | undefined;

    // Extract MCP server names
    const mcpServers = kiroConfig.mcpServers as Record<string, unknown> | undefined;
    const mcpServerNames = mcpServers ? Object.keys(mcpServers) : undefined;

    // Extract resource references
    const resources = kiroConfig.resources as string[] | undefined;

    nodes.push({
      id,
      type: 'agent',
      name,
      description: (manifest.description as string) || '',
      category: (manifest.category as string) || AGENT_CATEGORIES[name] || 'Other',
      platforms,
      profile: manifest.profile as 'haiku' | 'sonnet' | 'opus' | undefined,
      models,
      sources,
      model: models.claude?.model,
      tools: Array.isArray(claudeFrontmatter.tools)
        ? claudeFrontmatter.tools as string[]
        : undefined,
      mcpServers: mcpServerNames,
      trustedAgents,
      resources,
      sourcePath: `agents/${directoryName}/manifest.json`,
    });

    // Create delegation edges
    if (trustedAgents) {
      for (const target of trustedAgents) {
        edges.push({
          id: `edge:${name}->delegates->${target}`,
          source: id,
          target: `agent:${target}`,
          type: 'delegates-to',
        });
      }
    }

    // Create MCP server edges
    if (mcpServerNames) {
      for (const server of mcpServerNames) {
        edges.push({
          id: `edge:${name}->mcp->${server}`,
          source: id,
          target: `mcp:${server}`,
          type: 'uses-mcp',
        });
      }
    }

    // Create resource edges (skills and shared refs)
    if (resources) {
      for (const resource of resources) {
        // Match skill references like "skill://~/.kiro/skills/code-review-excellence/SKILL.md"
        const skillMatch = resource.match(/skills\/([^/]+)\//);
        if (skillMatch) {
          const skillName = skillMatch[1];
          if (skillName === '_shared') {
            // shared ref
            const sharedMatch = resource.match(/_shared\/([^/]+)$/);
            if (sharedMatch) {
              const refName = sharedMatch[1].replace('.md', '');
              edges.push({
                id: `edge:${name}->shared-ref->${refName}`,
                source: id,
                target: `shared-ref:${refName}`,
                type: 'uses-shared-ref',
              });
            }
          } else {
            edges.push({
              id: `edge:${name}->skill->${skillName}`,
              source: id,
              target: `skill:${skillName}`,
              type: 'uses-skill',
            });
          }
        }

        // Match file:// references to shared resources
        const fileSharedMatch = resource.match(/file:\/\/.*?skills\/_shared\/([^/]+)$/);
        if (fileSharedMatch) {
          const refName = fileSharedMatch[1].replace('.md', '');
          edges.push({
            id: `edge:${name}->shared-ref->${refName}`,
            source: id,
            target: `shared-ref:${refName}`,
            type: 'uses-shared-ref',
          });
        }
      }
    }
  }

  return { nodes, edges };
}

function extractSkills(): GraphNode[] {
  const grouped = new Map<string, {
    variants: Partial<Record<typeof SKILL_PLATFORMS[number], {
      path: string;
      content: string;
      metadata: Record<string, unknown>;
    }>>;
  }>();

  for (const platform of SKILL_PLATFORMS) {
    const platformRoot = join(SKILLS_DIR, platform);
    if (!existsSync(platformRoot)) continue;
    const dirs = readdirSync(platformRoot, { withFileTypes: true })
      .filter(entry => entry.isDirectory())
      .map(entry => entry.name);

    for (const directoryName of dirs) {
      const directory = join(platformRoot, directoryName);
      const skillPath = join(directory, 'SKILL.md');
      if (!existsSync(skillPath)) continue;
      const content = readFileSync(skillPath, 'utf-8');
      const metadata = parseFrontmatter(content);
      const name = typeof metadata.name === 'string' ? metadata.name : directoryName;
      const current = grouped.get(name) ?? { variants: {} };
      current.variants[platform] = {
        path: `skills/${platform}/${directoryName}/SKILL.md`,
        content,
        metadata,
      };
      grouped.set(name, current);
    }
  }

  return Array.from(grouped.entries()).sort(([left], [right]) => left.localeCompare(right)).map(
    ([name, { variants }]) => {
      const universal = variants.universal;
      const claude = variants.claude;
      const codex = variants.codex;
      const primary = universal ?? claude ?? codex;
      if (!primary) throw new Error(`Skill ${name} has no source`);

      const compatibility = universal
        ? 'universal'
        : claude && codex
          ? 'variants'
          : claude
            ? 'claude'
            : 'codex';
      const metadata = primary.metadata;
      const description = typeof metadata.description === 'string'
        ? metadata.description
        : '';
      const frontmatterMatch = primary.content.match(/^---\r?\n[\s\S]*?\r?\n---/);
      const body = frontmatterMatch
        ? primary.content.slice(frontmatterMatch[0].length)
        : primary.content;
      const plainBody = body
        .replace(/^#+\s*/gm, '')
        .replace(/\s+/g, ' ')
        .trim();
      const fmWhenToUse = metadata['when-to-use'] ?? metadata.whenToUse;
      const fmUserInvocable = metadata['user-invocable'] ?? metadata.userInvocable;
      const sources: GraphNode['sources'] = {};
      for (const [platform, variant] of Object.entries(variants)) {
        if (variant) sources[platform as keyof typeof sources] = variant.path;
      }

      return {
        id: `skill:${name}`,
        type: 'skill' as const,
        name,
        description,
        category: SKILL_CATEGORIES[name] || 'Other',
        platforms: universal
          ? ['claude', 'codex']
          : [
            ...(claude ? ['claude' as const] : []),
            ...(codex ? ['codex' as const] : []),
          ],
        compatibility,
        sources,
        sourcePath: primary.path,
        whenToUse: typeof fmWhenToUse === 'string' ? fmWhenToUse : undefined,
        excerpt: plainBody ? plainBody.slice(0, 300) : undefined,
        userInvocable: typeof fmUserInvocable === 'boolean' ? fmUserInvocable : undefined,
      };
    },
  );
}

function extractSharedRefs(): GraphNode[] {
  const nodes: GraphNode[] = [];

  if (!existsSync(SHARED_DIR)) return nodes;

  const files = readdirSync(SHARED_DIR).filter(f => f.endsWith('.md'));

  for (const file of files) {
    const refName = basename(file, '.md');
    const content = readFileSync(join(SHARED_DIR, file), 'utf-8');

    // Extract description from first heading or first paragraph
    let description = '';
    const headingMatch = content.match(/^#\s+(.+)/m);
    if (headingMatch) {
      description = headingMatch[1];
    }
    // Try to get a better description from first paragraph after heading
    const paraMatch = content.match(/^#.+\n+(?:>\s*)?(.+)/m);
    if (paraMatch) {
      description = paraMatch[1].replace(/^>\s*/, '').trim();
    }

    nodes.push({
      id: `shared-ref:${refName}`,
      type: 'shared-ref',
      name: refName,
      description,
      sourcePath: `skills/_shared/${file}`,
    });
  }

  return nodes;
}

function extractMcpServers(agentNodes: GraphNode[]): GraphNode[] {
  const serverSet = new Set<string>();

  for (const node of agentNodes) {
    if (node.mcpServers) {
      for (const server of node.mcpServers) {
        serverSet.add(server);
      }
    }
  }

  return Array.from(serverSet).map(name => ({
    id: `mcp:${name}`,
    type: 'mcp-server' as const,
    name,
    description: `MCP server: ${name}`,
  }));
}

interface LitmusCase {
  agent: string;
  case_id: string;
  model: string;
  input_tokens: number;
  output_tokens: number;
  cost_usd: number;
  duration_ms: number;
  detail_path?: string;
}

interface LitmusSummary {
  cases: LitmusCase[];
}

function buildEvalStats(): EvalStats {
  if (!existsSync(LITMUS_RESULTS_DIR)) {
    return { caseCount: 0, runCount: 0, byAgent: [], grandRuns: 0, grandTotalCostUsd: 0 };
  }

  const grouped: Record<string, LitmusCase[]> = {};
  const caseIDs = new Set<string>();
  let runCount = 0;
  for (const entry of readdirSync(LITMUS_RESULTS_DIR, { withFileTypes: true })) {
    if (!entry.isDirectory()) continue;
    const summaryPath = join(LITMUS_RESULTS_DIR, entry.name, 'summary.json');
    if (!existsSync(summaryPath)) continue;
    try {
      const summary = JSON.parse(readFileSync(summaryPath, 'utf-8')) as LitmusSummary;
      runCount++;
      for (const result of summary.cases || []) {
        const detailPath = result.detail_path && join(LITMUS_RESULTS_DIR, entry.name, result.detail_path);
        const detail = detailPath && existsSync(detailPath)
          ? JSON.parse(readFileSync(detailPath, 'utf-8')) as Partial<LitmusCase>
          : {};
        const complete = { ...result, ...detail, model: detail.model || result.model || 'unknown' };
        if (!grouped[complete.agent]) grouped[complete.agent] = [];
        grouped[complete.agent].push(complete as LitmusCase);
        caseIDs.add(`${complete.agent}/${complete.case_id}`);
      }
    } catch {
      continue;
    }
  }

  let grandRuns = 0;
  let grandTotalCostUsd = 0;
  const byAgent: EvalAgentSummary[] = [];

  for (const [agent, runs] of Object.entries(grouped).sort()) {
    const avgIn = Math.round(runs.reduce((s, r) => s + r.input_tokens, 0) / runs.length);
    const avgOut = Math.round(runs.reduce((s, r) => s + r.output_tokens, 0) / runs.length);
    const avgDurationS = Math.round(runs.reduce((s, r) => s + r.duration_ms / 1000, 0) / runs.length);
    const totalCostUsd = runs.reduce((s, r) => s + r.cost_usd, 0);
    grandRuns += runs.length;
    grandTotalCostUsd += totalCostUsd;
    byAgent.push({
      agent,
      model: runs.find(run => run.model !== 'unknown')?.model || 'unknown',
      runs: runs.length,
      avgIn,
      avgOut,
      avgDurationS,
      totalCostUsd,
    });
  }

  return { caseCount: caseIDs.size, runCount, byAgent, grandRuns, grandTotalCostUsd };
}

function buildStats(
  agentNodes: GraphNode[],
  skillNodes: GraphNode[],
  sharedRefNodes: GraphNode[],
  mcpNodes: GraphNode[],
  workflows: Workflow[],
): StatsData {
  const profileDistribution: Record<string, number> = {};
  const modelDistribution: Record<string, Record<string, number>> = {
    claude: {},
    codex: {},
    kiro: {},
  };
  const agentCategories: Record<string, number> = {};
  for (const node of agentNodes) {
    const profile = node.profile || 'unspecified';
    profileDistribution[profile] = (profileDistribution[profile] || 0) + 1;
    for (const [platform, model] of Object.entries(node.models || {})) {
      if (!model) continue;
      modelDistribution[platform][model.model] = (modelDistribution[platform][model.model] || 0) + 1;
    }
    const category = node.category || 'Other';
    agentCategories[category] = (agentCategories[category] || 0) + 1;
  }

  const skillCategories: Record<string, number> = {};
  for (const node of skillNodes) {
    const category = node.category || 'Other';
    skillCategories[category] = (skillCategories[category] || 0) + 1;
  }

  return {
    counts: {
      agents: agentNodes.length,
      skills: skillNodes.length,
      sharedRefs: sharedRefNodes.length,
      mcpServers: mcpNodes.length,
      workflows: workflows.length,
    },
    profileDistribution,
    modelDistribution,
    agentCategories,
    skillCategories,
    evals: buildEvalStats(),
  };
}

function main() {
  console.log('Extracting data from agents repo...');

  const { nodes: agentNodes, edges: agentEdges } = extractAgents();
  console.log(`  Found ${agentNodes.length} agents`);

  const skillNodes = extractSkills();
  console.log(`  Found ${skillNodes.length} skills`);

  const sharedRefNodes = extractSharedRefs();
  console.log(`  Found ${sharedRefNodes.length} shared references`);

  const mcpNodes = extractMcpServers(agentNodes);
  console.log(`  Found ${mcpNodes.length} MCP servers`);

  // Filter edges to only include those where both source and target exist
  const allNodeIds = new Set([
    ...agentNodes.map(n => n.id),
    ...skillNodes.map(n => n.id),
    ...sharedRefNodes.map(n => n.id),
    ...mcpNodes.map(n => n.id),
  ]);

  const validEdges = agentEdges.filter(e => allNodeIds.has(e.source) && allNodeIds.has(e.target));
  console.log(`  Found ${validEdges.length} valid edges (${agentEdges.length - validEdges.length} dangling edges removed)`);

  const workflows = getWorkflows();
  console.log(`  Found ${workflows.length} workflows`);

  const stats = buildStats(agentNodes, skillNodes, sharedRefNodes, mcpNodes, workflows);

  const graph: GraphData = {
    nodes: [...agentNodes, ...skillNodes, ...sharedRefNodes, ...mcpNodes],
    edges: validEdges,
    workflows,
    stats,
  };

  writeFileSync(OUTPUT, JSON.stringify(graph, null, 2));
  console.log(`\nWrote ${OUTPUT}`);
  console.log(`  Total: ${graph.nodes.length} nodes, ${graph.edges.length} edges, ${graph.workflows.length} workflows`);
}

function getWorkflows(): Workflow[] {
  return [
    {
      id: 'workflow:research',
      name: 'Research Pipeline',
      description: 'Research a topic with adversarial validation. Unverified findings loop back for another pass; only confirmed findings reach the summary.',
      orchestrator: 'crash-course',
      nodes: [
        { id: 'start', type: 'start', label: 'Start' },
        { id: 'research', type: 'process', label: 'Research', agent: 'researcher', description: 'Search external docs, APIs, libraries. Returns findings with source URLs.' },
        { id: 'validate', type: 'process', label: 'Validate', agent: 'research-validator', description: 'Cross-check cited sources. Classify each claim as CONFIRMED/UNVERIFIED/CONTRADICTED.' },
        { id: 'd1', type: 'decision', label: 'Confirmed?', description: 'Are the findings backed by their cited sources?' },
        { id: 'synthesize', type: 'process', label: 'Synthesize', description: 'Produce final summary using only CONFIRMED findings with citations.' },
        { id: 'end', type: 'end', label: 'Done' },
      ],
      edges: [
        { from: 'start', to: 'research' },
        { from: 'research', to: 'validate' },
        { from: 'validate', to: 'd1' },
        { from: 'd1', to: 'research', label: 'unverified', loop: true },
        { from: 'd1', to: 'synthesize', label: 'confirmed' },
        { from: 'synthesize', to: 'end' },
      ],
    },
    {
      id: 'workflow:code-review',
      name: 'Code Review Pipeline',
      description: 'Multi-phase review with a skeptic validator pass, branching to an APPROVE or BLOCK verdict.',
      orchestrator: 'code-reviewer',
      nodes: [
        { id: 'start', type: 'start', label: 'Start' },
        { id: 'diff', type: 'process', label: 'Gather Diff', agent: 'code-reviewer', description: 'Collect uncommitted/unpushed changes or PR diff' },
        { id: 'multi', type: 'process', label: 'Multi-lens Review', agent: 'code-reviewer', description: 'Review for correctness, security, design fit, testability, performance' },
        { id: 'skeptic', type: 'process', label: 'Skeptic Pass', agent: 'validator', description: 'Challenge review findings - are they real issues or false positives?' },
        { id: 'd1', type: 'decision', label: 'Issues found?', description: 'Did the skeptic pass confirm real blocking issues?' },
        { id: 'block', type: 'process', label: 'Report BLOCK', description: 'Aggregate and format findings into BLOCK verdict report' },
        { id: 'approve', type: 'process', label: 'Report APPROVE', description: 'Aggregate and format findings into APPROVE verdict report' },
        { id: 'end', type: 'end', label: 'Done' },
      ],
      edges: [
        { from: 'start', to: 'diff' },
        { from: 'diff', to: 'multi' },
        { from: 'multi', to: 'skeptic' },
        { from: 'skeptic', to: 'd1' },
        { from: 'd1', to: 'block', label: 'yes' },
        { from: 'd1', to: 'approve', label: 'no' },
        { from: 'block', to: 'end' },
        { from: 'approve', to: 'end' },
      ],
    },
    {
      id: 'workflow:implement-task',
      name: 'Task Implementation',
      description: 'End-to-end implementation from a task description or Jira ticket. Tests loop back to implementation until green.',
      orchestrator: 'implement-task',
      nodes: [
        { id: 'start', type: 'start', label: 'Start' },
        { id: 'fetch', type: 'process', label: 'Requirements + Scan', description: 'Read the task or Jira ticket and survey the codebase' },
        { id: 'plan', type: 'process', label: 'Plan', description: 'Break requirements into implementation tasks with dependency ordering' },
        { id: 'implement', type: 'process', label: 'Implement', agent: 'builder', description: 'Execute each task, delegating low-complexity subtasks' },
        { id: 'test', type: 'process', label: 'Test', agent: 'tester', description: 'Run tests and write missing tests' },
        { id: 'd1', type: 'decision', label: 'Tests pass?', description: 'Are all tests green?' },
        { id: 'review', type: 'process', label: 'Review', agent: 'code-reviewer', description: 'Merged code review across the full implementation' },
        { id: 'end', type: 'end', label: 'Done' },
      ],
      edges: [
        { from: 'start', to: 'fetch' },
        { from: 'fetch', to: 'plan' },
        { from: 'plan', to: 'implement' },
        { from: 'implement', to: 'test' },
        { from: 'test', to: 'd1' },
        { from: 'd1', to: 'implement', label: 'no', loop: true },
        { from: 'd1', to: 'review', label: 'yes' },
        { from: 'review', to: 'end' },
      ],
    },
  ];
}

main();
