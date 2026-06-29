import graphData from '../data/graph.json';
import { useNavigate } from 'react-router';
import { type GraphNode, type GraphData } from '../types';
import './NodeDetailPanel.css';

const data = graphData as GraphData;

interface Props {
  node: GraphNode;
  onClose: () => void;
}

export function NodeDetailPanel({ node, onClose }: Props) {
  const navigate = useNavigate();

  // Find connections
  const outgoing = data.edges.filter(e => e.source === node.id);
  const incoming = data.edges.filter(e => e.target === node.id);

  const connectedNodes = [
    ...outgoing.map(e => ({
      node: data.nodes.find(n => n.id === e.target),
      relation: e.type,
      direction: 'outgoing' as const,
    })),
    ...incoming.map(e => ({
      node: data.nodes.find(n => n.id === e.source),
      relation: e.type,
      direction: 'incoming' as const,
    })),
  ].filter(c => c.node != null);

  // Workflows that reference this agent
  const usedInWorkflows = node.type === 'agent'
    ? data.workflows.filter(wf => wf.nodes.some(n => n.agent === node.name))
    : [];

  const sourceUrl = node.sourcePath
    ? `https://github.com/liawedwa/agents/blob/main/${node.sourcePath}`
    : null;

  return (
    <div className="detail-panel-overlay" onClick={onClose}>
      <aside className="detail-panel" onClick={(e) => e.stopPropagation()}>
        <header className="detail-header">
          <div className="detail-header-top">
            <div className="detail-header-badges">
              <span className={`detail-type-badge type-${node.type}`}>{node.type}</span>
              {node.userInvocable && (
                <span className="detail-invocable-badge">user-invocable</span>
              )}
            </div>
            <button className="detail-close" onClick={onClose} aria-label="Close">×</button>
          </div>
          <h2 className="detail-name">{node.name}</h2>
          {node.category && <span className="detail-category">{node.category}</span>}
        </header>

        <section className="detail-body">
          <p className="detail-description">{node.description}</p>

          {node.type === 'skill' && node.excerpt && (
            <div className="detail-field">
              <span className="detail-field-label">About</span>
              <p className="detail-excerpt">{node.excerpt}</p>
            </div>
          )}

          {node.model && (
            <div className="detail-field">
              <span className="detail-field-label">Model</span>
              <span className="detail-field-value">{node.model}</span>
            </div>
          )}

          {node.trustedAgents && node.trustedAgents.length > 0 && (
            <div className="detail-field">
              <span className="detail-field-label">Delegates to</span>
              <div className="detail-field-tags">
                {node.trustedAgents.map(a => (
                  <span key={a} className="detail-tag tag-agent">{a}</span>
                ))}
              </div>
            </div>
          )}

          {node.mcpServers && node.mcpServers.length > 0 && (
            <div className="detail-field">
              <span className="detail-field-label">MCP Servers</span>
              <div className="detail-field-tags">
                {node.mcpServers.map(s => (
                  <span key={s} className="detail-tag tag-mcp">{s}</span>
                ))}
              </div>
            </div>
          )}

          {node.tools && node.tools.length > 0 && (
            <div className="detail-field">
              <span className="detail-field-label">Tools ({node.tools.length})</span>
              <div className="detail-field-tags">
                {node.tools.map(t => (
                  <span key={t} className="detail-tag">{t.replace(/^@[^/]+\//, '')}</span>
                ))}
              </div>
            </div>
          )}

          {node.resources && node.resources.length > 0 && (
            <div className="detail-field">
              <span className="detail-field-label">Resources</span>
              <div className="detail-field-tags">
                {node.resources.map(r => (
                  <span key={r} className="detail-tag">{r.replace(/.*\//, '')}</span>
                ))}
              </div>
            </div>
          )}

          {connectedNodes.length > 0 && (
            <div className="detail-field">
              <span className="detail-field-label">Connections ({connectedNodes.length})</span>
              <div className="detail-connections">
                {connectedNodes.map(({ node: connNode, relation, direction }) => (
                  <div key={`${connNode!.id}-${relation}`} className="detail-connection">
                    <span className={`detail-tag tag-${connNode!.type}`}>{connNode!.name}</span>
                    <span className="connection-relation">
                      {direction === 'outgoing' ? '→' : '←'} {relation.replace(/-/g, ' ')}
                    </span>
                  </div>
                ))}
              </div>
            </div>
          )}

          {usedInWorkflows.length > 0 && (
            <div className="detail-field">
              <span className="detail-field-label">Used in workflows</span>
              <div className="detail-workflow-links">
                {usedInWorkflows.map(wf => (
                  <button
                    key={wf.id}
                    className="detail-workflow-link"
                    onClick={() => navigate('/workflows')}
                  >
                    {wf.name}
                  </button>
                ))}
              </div>
            </div>
          )}
        </section>

        {sourceUrl && (
          <footer className="detail-footer">
            <a
              href={sourceUrl}
              target="_blank"
              rel="noopener noreferrer"
              className="detail-source-link"
            >
              View source →
            </a>
          </footer>
        )}
      </aside>
    </div>
  );
}
