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
  'team-lead': 'Orchestration',
  'context-curator': 'Orchestration',
  'team-leader': 'Orchestration',
  'builder': 'Implementation',
  'developer': 'Implementation',
  'superhuman': 'Implementation',
  'tester': 'Quality Assurance',
  'validator': 'Quality Assurance',
  'code-reviewer': 'Quality Assurance',
  'security-reviewer': 'Quality Assurance',
  'glide-code-reviewer': 'Quality Assurance',
  'researcher': 'Research',
  'research-validator': 'Research',
  'research-summarizer': 'Research',
  'documenter': 'Documentation',
  'valkey-glide-implementor': 'Valkey & GLIDE',
  'explore': 'Research',
};

const SKILL_CATEGORIES: Record<string, string> = {
  'review-code': 'Code Review',
  'review-pr': 'Code Review',
  'review-cookbook-pr': 'Code Review',
  'multi-discipline-review': 'Code Review',
  'code-review-excellence': 'Code Review',
  'requesting-code-review': 'Code Review',
  'receiving-code-review': 'Code Review',
  'review-addressed-comments': 'Code Review',
  'write-pr': 'Writing & Documentation',
  'write-pr-comments': 'Writing & Documentation',
  'write-narrative': 'Writing & Documentation',
  'humanizer': 'Writing & Documentation',
  'pr-comment-humanizer': 'Writing & Documentation',
  'implement-jira': 'Development Workflows',
  'subagent-driven-development': 'Development Workflows',
  'systematic-debugging': 'Development Workflows',
  'test-driven-development': 'Development Workflows',
  'brainstorming': 'Development Workflows',
  'verification-before-completion': 'Development Workflows',
  'finishing-a-development-branch': 'Development Workflows',
  'writing-plans': 'Planning & Orchestration',
  'executing-plans': 'Planning & Orchestration',
  'dispatching-parallel-agents': 'Planning & Orchestration',
  'using-git-worktrees': 'Planning & Orchestration',
  'glide-skill': 'Valkey & GLIDE',
  'test-valkey': 'Valkey & GLIDE',
  'valkey-spike': 'Valkey & GLIDE',
  'diagram-valkey-flow': 'Valkey & GLIDE',
  'implement-cookbook': 'Valkey & GLIDE',
  'check-valkey-search-compatibility': 'Valkey & GLIDE',
  'create-skill': 'Meta',
  'update-skill': 'Meta',
  'update-agent': 'Meta',
  'writing-skills': 'Meta',
  'find-skills': 'Meta',
  'using-superpowers': 'Meta',
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
      id: 'workflow:team-lead',
      name: 'Team Lead Pipeline',
      description: 'Full implementation workflow from spec to merged code. Context and research run in parallel before each build; both review stages can loop back.',
      orchestrator: 'team-lead',
      nodes: [
        { id: 'start', type: 'start', label: 'Start' },
        { id: 'explore', type: 'process', label: 'Explore', agent: 'explore', description: 'Survey codebase: existing patterns, relevant files, interfaces, conventions' },
        { id: 'plan', type: 'process', label: 'Plan', agent: 'team-lead', description: 'Extract all tasks from spec findings, create TODO, classify complexity' },
        { id: 'context', type: 'process', label: 'Gather Context', agent: 'context-curator', description: 'Fetch relevant memories from Valkey + Obsidian for the builder' },
        { id: 'research', type: 'process', label: 'Research', agent: 'researcher', description: 'Investigate unfamiliar APIs/libraries before implementation', optional: true },
        { id: 'build', type: 'process', label: 'Build', agent: 'builder', description: 'Implement the task (or superhuman for complex tasks)' },
        { id: 'validate', type: 'process', label: 'Validate Spec', agent: 'validator', description: 'Verify requirements are met - nothing missing, nothing extra' },
        { id: 'd1', type: 'decision', label: 'Spec met?', description: 'Did the builder satisfy every requirement?' },
        { id: 'review', type: 'process', label: 'Review Quality', agent: 'code-reviewer', description: 'Check correctness, codebase alignment, maintainability' },
        { id: 'd2', type: 'decision', label: 'Approved?', description: 'Is the code correct and well-built?' },
        { id: 'document', type: 'process', label: 'Document', agent: 'documenter', description: 'Generate documentation for completed features', optional: true },
        { id: 'end', type: 'end', label: 'Done' },
      ],
      edges: [
        { from: 'start', to: 'explore' },
        { from: 'explore', to: 'plan' },
        { from: 'plan', to: 'context' },
        { from: 'plan', to: 'research' },
        { from: 'context', to: 'build' },
        { from: 'research', to: 'build' },
        { from: 'build', to: 'validate' },
        { from: 'validate', to: 'd1' },
        { from: 'd1', to: 'build', label: 'no', loop: true },
        { from: 'd1', to: 'review', label: 'yes' },
        { from: 'review', to: 'd2' },
        { from: 'd2', to: 'build', label: 'no', loop: true },
        { from: 'd2', to: 'document', label: 'yes' },
        { from: 'document', to: 'end' },
      ],
    },
    {
      id: 'workflow:research',
      name: 'Research Pipeline',
      description: 'Research a topic with adversarial validation. Unverified findings loop back for another pass; only confirmed findings reach the summary.',
      orchestrator: 'research-summarizer',
      nodes: [
        { id: 'start', type: 'start', label: 'Start' },
        { id: 'research', type: 'process', label: 'Research', agent: 'researcher', description: 'Search external docs, APIs, libraries. Returns findings with source URLs.' },
        { id: 'validate', type: 'process', label: 'Validate', agent: 'research-validator', description: 'Cross-check cited sources. Classify each claim as CONFIRMED/UNVERIFIED/CONTRADICTED.' },
        { id: 'd1', type: 'decision', label: 'Confirmed?', description: 'Are the findings backed by their cited sources?' },
        { id: 'synthesize', type: 'process', label: 'Synthesize', agent: 'research-summarizer', description: 'Produce final summary using only CONFIRMED findings with citations.' },
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
        { id: 'block', type: 'process', label: 'Report BLOCK', agent: 'documenter', description: 'Aggregate and format findings into BLOCK verdict report' },
        { id: 'approve', type: 'process', label: 'Report APPROVE', agent: 'documenter', description: 'Aggregate and format findings into APPROVE verdict report' },
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
      id: 'workflow:implement-jira',
      name: 'Jira Implementation',
      description: 'End-to-end implementation from a Jira ticket. Tests loop back to implementation until green.',
      orchestrator: 'implement-jira',
      nodes: [
        { id: 'start', type: 'start', label: 'Start' },
        { id: 'fetch', type: 'process', label: 'Fetch Requirements + Scan', agent: 'explore', description: 'Read Jira ticket and survey codebase in one pass' },
        { id: 'plan', type: 'process', label: 'Plan', agent: 'superhuman', description: 'Break requirements into implementation tasks with dependency ordering' },
        { id: 'context', type: 'process', label: 'Gather Context', agent: 'context-curator', description: 'Load relevant memories and prior decisions' },
        { id: 'implement', type: 'process', label: 'Implement', agent: 'builder', description: 'Execute each task (with complexity routing to superhuman if needed)' },
        { id: 'test', type: 'process', label: 'Test', agent: 'tester', description: 'Run tests and write missing tests' },
        { id: 'd1', type: 'decision', label: 'Tests pass?', description: 'Are all tests green?' },
        { id: 'review', type: 'process', label: 'Review', agent: 'code-reviewer', description: 'Merged code review across the full implementation' },
        { id: 'end', type: 'end', label: 'Done' },
      ],
      edges: [
        { from: 'start', to: 'fetch' },
        { from: 'fetch', to: 'plan' },
        { from: 'plan', to: 'context' },
        { from: 'context', to: 'implement' },
        { from: 'implement', to: 'test' },
        { from: 'test', to: 'd1' },
        { from: 'd1', to: 'implement', label: 'no', loop: true },
        { from: 'd1', to: 'review', label: 'yes' },
        { from: 'review', to: 'end' },
      ],
    },
    {
      id: 'workflow:security-review',
      name: 'Security Review',
      description: 'Threat-model-driven security analysis anchored to CWE taxonomy.',
      orchestrator: 'security-reviewer',
      nodes: [
        { id: 'start', type: 'start', label: 'Start' },
        { id: 'threat', type: 'process', label: 'Threat Model', agent: 'security-reviewer', description: 'Identify attack surface: inputs, boundaries, data flows' },
        { id: 'cwe', type: 'process', label: 'CWE Analysis', agent: 'security-reviewer', description: 'Check for injection, access control, secrets, crypto, SSRF, path traversal' },
        { id: 'd1', type: 'decision', label: 'Vulns found?', description: 'Did analysis surface any vulnerabilities?' },
        { id: 'report', type: 'process', label: 'Report Findings', agent: 'security-reviewer', description: 'Report vulnerabilities with CWE IDs, severity, and remediation guidance' },
        { id: 'clear', type: 'process', label: 'Report Clear', agent: 'security-reviewer', description: 'No blocking vulnerabilities found' },
        { id: 'end', type: 'end', label: 'Done' },
      ],
      edges: [
        { from: 'start', to: 'threat' },
        { from: 'threat', to: 'cwe' },
        { from: 'cwe', to: 'd1' },
        { from: 'd1', to: 'report', label: 'yes' },
        { from: 'd1', to: 'clear', label: 'no' },
        { from: 'report', to: 'end' },
        { from: 'clear', to: 'end' },
      ],
    },
  ];
}

main();
