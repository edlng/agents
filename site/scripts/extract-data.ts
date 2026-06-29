import { readFileSync, readdirSync, writeFileSync, existsSync } from 'fs';
import { join, basename, resolve } from 'path';
import { parse as parseYaml } from 'yaml';

const ROOT = resolve(import.meta.dirname, '..', '..');
const AGENTS_DIR = join(ROOT, 'agents');
const SKILLS_DIR = join(ROOT, 'skills');
const SHARED_DIR = join(SKILLS_DIR, '_shared');
const OUTPUT = join(import.meta.dirname, '..', 'src', 'data', 'graph.json');
const TOKEN_USAGE_FILE = join(ROOT, 'evals', 'metrics', 'token_usage.jsonl');
const PROMPTFOO_FULL = join(ROOT, 'promptfooconfig.yaml');
const PROMPTFOO_SMOKE = join(ROOT, 'promptfooconfig.smoke.yaml');

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
  fullTestCount: number;
  smokeTestCount: number;
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
  modelDistribution: Record<string, number>;
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

function extractAgents(): { nodes: GraphNode[]; edges: GraphEdge[] } {
  const nodes: GraphNode[] = [];
  const edges: GraphEdge[] = [];

  const files = readdirSync(AGENTS_DIR).filter(f => f.endsWith('.json') && f !== 'agent_config.json.example');

  for (const file of files) {
    const content = readFileSync(join(AGENTS_DIR, file), 'utf-8');
    let config: Record<string, unknown>;
    try {
      config = JSON.parse(content);
    } catch {
      console.warn(`Skipping invalid JSON: ${file}`);
      continue;
    }

    const name = config.name as string;
    const id = `agent:${name}`;

    // Extract trustedAgents from toolsSettings
    const toolsSettings = config.toolsSettings as Record<string, Record<string, unknown>> | undefined;
    const trustedAgents = toolsSettings?.subagent?.trustedAgents as string[] | undefined;

    // Extract MCP server names
    const mcpServers = config.mcpServers as Record<string, unknown> | undefined;
    const mcpServerNames = mcpServers ? Object.keys(mcpServers) : undefined;

    // Extract resource references
    const resources = config.resources as string[] | undefined;

    nodes.push({
      id,
      type: 'agent',
      name,
      description: (config.description as string) || '',
      category: AGENT_CATEGORIES[name] || 'Other',
      model: config.model as string | undefined,
      tools: config.allowedTools as string[] | undefined,
      mcpServers: mcpServerNames,
      trustedAgents,
      resources,
      sourcePath: `agents/${file}`,
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
  const nodes: GraphNode[] = [];

  const dirs = readdirSync(SKILLS_DIR, { withFileTypes: true })
    .filter(d => d.isDirectory() && d.name !== '_shared' && d.name !== '.DS_Store');

  for (const dir of dirs) {
    const skillMdPath = join(SKILLS_DIR, dir.name, 'SKILL.md');
    // Some skills use lowercase skill.md
    const skillMdPathAlt = join(SKILLS_DIR, dir.name, 'skill.md');

    let mdPath = '';
    if (existsSync(skillMdPath)) {
      mdPath = skillMdPath;
    } else if (existsSync(skillMdPathAlt)) {
      mdPath = skillMdPathAlt;
    } else {
      continue;
    }

    // Resolve the real on-disk filename (the case-insensitive filesystem
    // makes both SKILL.md and skill.md "exist", so read the directory to
    // get the actual casing for the source path we report).
    const dirEntries = readdirSync(join(SKILLS_DIR, dir.name));
    const mdFile = dirEntries.find(f => f.toLowerCase() === 'skill.md') ?? basename(mdPath);

    const content = readFileSync(mdPath, 'utf-8');

    // Extract YAML frontmatter
    let name = dir.name;
    let description = '';
    let whenToUse: string | undefined;
    let userInvocable: boolean | undefined;

    const frontmatterMatch = content.match(/^---\n([\s\S]*?)\n---/);
    if (frontmatterMatch) {
      try {
        const fm = parseYaml(frontmatterMatch[1]);
        if (fm.name) name = fm.name;
        if (fm.description) description = fm.description;
        // Only set when discoverable from frontmatter; otherwise omit.
        const fmWhenToUse = fm['when-to-use'] ?? fm.whenToUse;
        if (typeof fmWhenToUse === 'string') whenToUse = fmWhenToUse;
        const fmUserInvocable = fm['user-invocable'] ?? fm.userInvocable;
        if (typeof fmUserInvocable === 'boolean') userInvocable = fmUserInvocable;
      } catch {
        // Fall back to directory name
      }
    }

    // If no description from frontmatter, try first paragraph after heading
    if (!description) {
      const headingMatch = content.match(/^#\s+.+\n+(.+)/m);
      if (headingMatch) {
        description = headingMatch[1].trim();
      }
    }

    // Body excerpt: first ~300 chars after the frontmatter block,
    // stripped of heading markers and collapsed whitespace.
    const body = frontmatterMatch
      ? content.slice(frontmatterMatch[0].length)
      : content;
    const plainBody = body
      .replace(/^#+\s*/gm, '')
      .replace(/\s+/g, ' ')
      .trim();
    const excerpt = plainBody ? plainBody.slice(0, 300) : undefined;

    nodes.push({
      id: `skill:${dir.name}`,
      type: 'skill',
      name,
      description,
      category: SKILL_CATEGORIES[dir.name] || 'Other',
      sourcePath: `skills/${dir.name}/${mdFile}`,
      whenToUse,
      excerpt,
      userInvocable,
    });
  }

  return nodes;
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

interface TokenUsageEntry {
  agent: string;
  model: string;
  input_tokens: number;
  output_tokens: number;
  total_cost_usd: number;
  duration_s?: number;
}

// Count `description:` lines in a promptfoo config (matches `grep -c description:`).
// Missing file -> 0.
function countDescriptions(file: string): number {
  if (!existsSync(file)) return 0;
  const content = readFileSync(file, 'utf-8');
  return content.split('\n').filter(line => line.includes('description:')).length;
}

// Aggregate per-agent eval metrics from token_usage.jsonl.
// Logic ported from evals/scripts/cost-summary.js. Missing/empty -> zeroed.
function buildEvalStats(): EvalStats {
  const fullTestCount = countDescriptions(PROMPTFOO_FULL);
  const smokeTestCount = countDescriptions(PROMPTFOO_SMOKE);

  if (!existsSync(TOKEN_USAGE_FILE)) {
    return { fullTestCount, smokeTestCount, byAgent: [], grandRuns: 0, grandTotalCostUsd: 0 };
  }

  const content = readFileSync(TOKEN_USAGE_FILE, 'utf-8').trim();
  if (!content) {
    return { fullTestCount, smokeTestCount, byAgent: [], grandRuns: 0, grandTotalCostUsd: 0 };
  }

  const entries: TokenUsageEntry[] = content
    .split('\n')
    .filter(Boolean)
    .map(line => JSON.parse(line) as TokenUsageEntry);

  // Group by agent
  const grouped: Record<string, TokenUsageEntry[]> = {};
  for (const entry of entries) {
    if (!grouped[entry.agent]) grouped[entry.agent] = [];
    grouped[entry.agent].push(entry);
  }

  let grandRuns = 0;
  let grandTotalCostUsd = 0;
  const byAgent: EvalAgentSummary[] = [];

  for (const [agent, runs] of Object.entries(grouped).sort()) {
    const avgIn = Math.round(runs.reduce((s, r) => s + r.input_tokens, 0) / runs.length);
    const avgOut = Math.round(runs.reduce((s, r) => s + r.output_tokens, 0) / runs.length);
    const avgDurationS = Math.round(runs.reduce((s, r) => s + (r.duration_s || 0), 0) / runs.length);
    const totalCostUsd = runs.reduce((s, r) => s + r.total_cost_usd, 0);
    grandRuns += runs.length;
    grandTotalCostUsd += totalCostUsd;
    byAgent.push({
      agent,
      model: runs[0].model,
      runs: runs.length,
      avgIn,
      avgOut,
      avgDurationS,
      totalCostUsd,
    });
  }

  return { fullTestCount, smokeTestCount, byAgent, grandRuns, grandTotalCostUsd };
}

function buildStats(
  agentNodes: GraphNode[],
  skillNodes: GraphNode[],
  sharedRefNodes: GraphNode[],
  mcpNodes: GraphNode[],
  workflows: Workflow[],
): StatsData {
  const modelDistribution: Record<string, number> = {};
  const agentCategories: Record<string, number> = {};
  for (const node of agentNodes) {
    const model = node.model || 'unspecified';
    modelDistribution[model] = (modelDistribution[model] || 0) + 1;
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
        { id: 'block', type: 'process', label: 'Report BLOCK', agent: 'code-reviewer', description: 'Issue BLOCK verdict with evidence-backed findings grouped by severity' },
        { id: 'approve', type: 'process', label: 'Report APPROVE', agent: 'code-reviewer', description: 'Issue APPROVE verdict - code is correct and meets requirements' },
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
      orchestrator: 'team-lead',
      nodes: [
        { id: 'start', type: 'start', label: 'Start' },
        { id: 'fetch', type: 'process', label: 'Fetch Requirements', agent: 'team-lead', description: 'Read Jira ticket, extract acceptance criteria and constraints' },
        { id: 'scan', type: 'process', label: 'Scan Codebase', agent: 'explore', description: 'Find relevant files, existing patterns, interfaces to implement' },
        { id: 'plan', type: 'process', label: 'Plan', agent: 'team-lead', description: 'Break requirements into implementation tasks with dependency ordering' },
        { id: 'context', type: 'process', label: 'Gather Context', agent: 'context-curator', description: 'Load relevant memories and prior decisions' },
        { id: 'implement', type: 'process', label: 'Implement', agent: 'builder', description: 'Execute each task (with complexity routing to superhuman if needed)' },
        { id: 'test', type: 'process', label: 'Test', agent: 'tester', description: 'Run tests and write missing tests' },
        { id: 'd1', type: 'decision', label: 'Tests pass?', description: 'Are all tests green?' },
        { id: 'review', type: 'process', label: 'Review', agent: 'code-reviewer', description: 'Merged code review across the full implementation' },
        { id: 'end', type: 'end', label: 'Done' },
      ],
      edges: [
        { from: 'start', to: 'fetch' },
        { from: 'fetch', to: 'scan' },
        { from: 'scan', to: 'plan' },
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
