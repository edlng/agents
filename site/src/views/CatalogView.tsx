import { useState, useMemo, useEffect } from 'react';
import { useSearchParams } from 'react-router';
import graphData from '../data/graph.json';
import { type GraphNode, type NodeType } from '../types';
import { NodeDetailPanel } from '../components/NodeDetailPanel';
import './CatalogView.css';

const data = graphData as { nodes: GraphNode[]; edges: typeof graphData.edges };

const TYPE_LABELS: Record<NodeType, string> = {
  agent: 'Agents',
  skill: 'Skills',
  'shared-ref': 'Shared Refs',
  'mcp-server': 'MCP Servers',
};

const TYPE_ORDER: NodeType[] = ['agent', 'skill', 'shared-ref', 'mcp-server'];

function TypeBadge({ type }: { type: NodeType }) {
  return <span className={`type-badge type-${type}`}>{type}</span>;
}

function Card({ node, onSelect }: { node: GraphNode; onSelect: (node: GraphNode) => void }) {
  return (
    <div className={`card card-${node.type}`} onClick={() => onSelect(node)}>
      <div className="card-header">
        <h3 className="card-name">{node.name}</h3>
        <TypeBadge type={node.type} />
      </div>
      {node.category && <span className="card-category">{node.category}</span>}
      <p className="card-description">{node.description}</p>
      {node.model && <span className="card-model">{node.model}</span>}
    </div>
  );
}

export function CatalogView() {
  const [search, setSearch] = useState('');
  const [activeTypes, setActiveTypes] = useState<Set<NodeType>>(new Set(TYPE_ORDER));
  const [selectedNode, setSelectedNode] = useState<GraphNode | null>(null);
  const [searchParams, setSearchParams] = useSearchParams();

  // Open detail panel for ?node= param on mount
  useEffect(() => {
    const nodeId = searchParams.get('node');
    if (nodeId) {
      const found = data.nodes.find(n => n.id === nodeId);
      if (found) setSelectedNode(found);
    }
  // Only run on mount
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const filteredNodes = useMemo(() => {
    const term = search.toLowerCase();
    return data.nodes.filter(node => {
      if (!activeTypes.has(node.type)) return false;
      if (term && !node.name.toLowerCase().includes(term) && !node.description.toLowerCase().includes(term)) {
        return false;
      }
      return true;
    });
  }, [search, activeTypes]);

  const groupedNodes = useMemo(() => {
    const groups: Partial<Record<NodeType, GraphNode[]>> = {};
    for (const node of filteredNodes) {
      if (!groups[node.type]) groups[node.type] = [];
      groups[node.type]!.push(node);
    }
    // Sort within groups
    for (const type of TYPE_ORDER) {
      if (groups[type]) {
        groups[type]!.sort((a, b) => a.name.localeCompare(b.name));
      }
    }
    return groups;
  }, [filteredNodes]);

  function toggleType(type: NodeType) {
    setActiveTypes(prev => {
      const next = new Set(prev);
      if (next.has(type)) {
        next.delete(type);
      } else {
        next.add(type);
      }
      return next;
    });
  }

  return (
    <div className="catalog-view">
      <div className="catalog-controls">
        <input
          type="text"
          className="search-input"
          placeholder="Search agents, skills, refs..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        <div className="type-filters">
          {TYPE_ORDER.map(type => (
            <button
              key={type}
              className={`type-filter ${activeTypes.has(type) ? 'active' : ''} filter-${type}`}
              onClick={() => toggleType(type)}
            >
              {TYPE_LABELS[type]}
              <span className="filter-count">
                {data.nodes.filter(n => n.type === type).length}
              </span>
            </button>
          ))}
        </div>
      </div>

      <div className="catalog-results">
        <span className="result-count">{filteredNodes.length} items</span>
      </div>

      <div className="catalog-groups">
        {TYPE_ORDER.map(type => {
          const nodes = groupedNodes[type];
          if (!nodes || nodes.length === 0) return null;
          return (
            <section key={type} className="catalog-group">
              <h2 className="group-title">{TYPE_LABELS[type]}</h2>
              <div className="card-grid">
                {nodes.map(node => (
                  <Card key={node.id} node={node} onSelect={setSelectedNode} />
                ))}
              </div>
            </section>
          );
        })}
      </div>

      {selectedNode && (
        <NodeDetailPanel
          node={selectedNode}
          onClose={() => {
            setSelectedNode(null);
            if (searchParams.has('node')) {
              setSearchParams(prev => {
                const next = new URLSearchParams(prev);
                next.delete('node');
                return next;
              });
            }
          }}
        />
      )}
    </div>
  );
}
