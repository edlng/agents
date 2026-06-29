import { useNavigate } from 'react-router';
import graphData from '../data/graph.json';
import { type GraphData } from '../types';
import './OverviewView.css';

const data = graphData as GraphData;
const { stats } = data;

const STAT_CHIPS = [
  { key: 'agents',      label: 'Agents',      color: '#58a6ff', count: stats.counts.agents },
  { key: 'skills',      label: 'Skills',      color: '#3fb950', count: stats.counts.skills },
  { key: 'sharedRefs',  label: 'Shared Refs', color: '#d29922', count: stats.counts.sharedRefs },
  { key: 'mcpServers',  label: 'MCP Servers', color: '#bc8cff', count: stats.counts.mcpServers },
  { key: 'workflows',   label: 'Workflows',   color: '#8b949e', count: stats.counts.workflows },
] as const;

function formatCost(usd: number): string {
  return `$${usd.toFixed(4)}`;
}

function formatTokens(n: number): string {
  if (n >= 1000) return `${(n / 1000).toFixed(1)}k`;
  return String(n);
}

function ModelBar({ model, count, max }: { model: string; count: number; max: number }) {
  const pct = max > 0 ? Math.round((count / max) * 100) : 0;
  return (
    <div className="model-row">
      <span className="model-label">{model}</span>
      <div className="model-bar-track">
        <div className="model-bar-fill" style={{ width: `${pct}%` }} />
      </div>
      <span className="model-count">{count}</span>
    </div>
  );
}

export function OverviewView() {
  const navigate = useNavigate();

  const modelEntries = Object.entries(stats.modelDistribution).sort((a, b) => b[1] - a[1]);
  const maxModelCount = modelEntries.length > 0 ? modelEntries[0][1] : 1;

  const agentCategoryEntries = Object.entries(stats.agentCategories).sort((a, b) => b[1] - a[1]);
  const skillCategoryEntries = Object.entries(stats.skillCategories).sort((a, b) => b[1] - a[1]);

  const { evals } = stats;
  const hasEvalData = evals.byAgent.length > 0;

  return (
    <div className="page-container overview-view">

      {/* Hero counts */}
      <section className="overview-section">
        <div className="hero-chips">
          {STAT_CHIPS.map(chip => (
            <div
              key={chip.key}
              className="stat-chip"
              style={{ borderTopColor: chip.color }}
            >
              <span className="stat-number" style={{ color: chip.color }}>{chip.count}</span>
              <span className="stat-label">{chip.label}</span>
            </div>
          ))}
        </div>
      </section>

      {/* Model distribution */}
      <section className="overview-section">
        <h2 className="section-title">Agent Models</h2>
        <div className="model-bars">
          {modelEntries.map(([model, count]) => (
            <ModelBar key={model} model={model} count={count} max={maxModelCount} />
          ))}
        </div>
      </section>

      {/* Category breakdown */}
      <section className="overview-section">
        <div className="categories-grid">
          <div className="category-column">
            <h3 className="column-title">Agent categories</h3>
            <ul className="category-list">
              {agentCategoryEntries.map(([cat, count]) => (
                <li key={cat} className="category-item">
                  <span className="category-name">{cat}</span>
                  <span className="category-count">{count}</span>
                </li>
              ))}
            </ul>
          </div>
          <div className="category-column">
            <h3 className="column-title">Skill categories</h3>
            <ul className="category-list">
              {skillCategoryEntries.map(([cat, count]) => (
                <li key={cat} className="category-item">
                  <span className="category-name">{cat}</span>
                  <span className="category-count">{count}</span>
                </li>
              ))}
            </ul>
          </div>
        </div>
      </section>

      {/* Eval & cost panel */}
      <section className="overview-section">
        <h2 className="section-title">Evaluations</h2>
        <div className="eval-meta">
          <span className="eval-stat">
            <span className="eval-stat-value">{evals.fullTestCount}</span>
            <span className="eval-stat-label">full tests</span>
          </span>
          <span className="eval-meta-sep" />
          <span className="eval-stat">
            <span className="eval-stat-value">{evals.smokeTestCount}</span>
            <span className="eval-stat-label">smoke tests</span>
          </span>
        </div>

        {hasEvalData ? (
          <div className="eval-table-wrap">
            <table className="eval-table">
              <thead>
                <tr>
                  <th>Agent</th>
                  <th>Model</th>
                  <th className="num-col">Runs</th>
                  <th className="num-col">Avg In</th>
                  <th className="num-col">Avg Out</th>
                  <th className="num-col">Avg Duration</th>
                  <th className="num-col">Total Cost</th>
                </tr>
              </thead>
              <tbody>
                {evals.byAgent.map(row => (
                  <tr key={row.agent}>
                    <td className="agent-cell">{row.agent}</td>
                    <td className="model-cell">{row.model}</td>
                    <td className="num-col">{row.runs}</td>
                    <td className="num-col">{formatTokens(row.avgIn)}k</td>
                    <td className="num-col">{formatTokens(row.avgOut)}k</td>
                    <td className="num-col">{row.avgDurationS}s</td>
                    <td className="num-col cost-cell">{formatCost(row.totalCostUsd)}</td>
                  </tr>
                ))}
              </tbody>
              <tfoot>
                <tr className="grand-total-row">
                  <td colSpan={2} className="total-label">Total</td>
                  <td className="num-col">{evals.grandRuns}</td>
                  <td className="num-col" />
                  <td className="num-col" />
                  <td className="num-col" />
                  <td className="num-col cost-cell">{formatCost(evals.grandTotalCostUsd)}</td>
                </tr>
              </tfoot>
            </table>
          </div>
        ) : (
          <p className="eval-empty">Run <code>make eval</code> to populate eval metrics.</p>
        )}
      </section>

      {/* Quick links */}
      <section className="overview-section">
        <div className="quick-links">
          <button className="quick-link-card" onClick={() => navigate('/catalog')}>
            <span className="quick-link-label">Browse Catalog</span>
            <span className="quick-link-arrow">→</span>
          </button>
          <button className="quick-link-card" onClick={() => navigate('/workflows')}>
            <span className="quick-link-label">View Workflows</span>
            <span className="quick-link-arrow">→</span>
          </button>
        </div>
      </section>

    </div>
  );
}
