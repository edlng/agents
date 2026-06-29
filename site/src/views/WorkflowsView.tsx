import { useState, useMemo, useCallback, useEffect } from 'react';
import { useNavigate } from 'react-router';
import {
  ReactFlow,
  Background,
  Controls,
  Handle,
  Position,
  type Node,
  type Edge,
  type NodeProps,
  type NodeTypes,
  useNodesState,
  useEdgesState,
  ReactFlowProvider,
  MarkerType,
  type NodeMouseHandler,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import dagre from 'dagre';
import graphData from '../data/graph.json';
import { type GraphData, type Workflow, type FlowNode as WFNode } from '../types';
import './WorkflowsView.css';

const data = graphData as GraphData;

// ---- Custom node shapes ----

function TerminalNode({ data }: NodeProps) {
  const d = data as { label: string };
  return (
    <div className="fc-terminal">
      <Handle type="target" position={Position.Top} id="top" />
      <Handle type="source" position={Position.Bottom} id="bottom" />
      {d.label}
    </div>
  );
}

function ProcessNode({ data, selected }: NodeProps) {
  const d = data as { label: string; agent?: string; optional?: boolean };
  return (
    <div className={`fc-process ${d.optional ? 'optional' : ''} ${selected ? 'selected' : ''}`}>
      <Handle type="target" position={Position.Top} id="top" />
      <Handle type="target" position={Position.Right} id="right-t" />
      <Handle type="target" position={Position.Left} id="left-t" />
      <Handle type="source" position={Position.Bottom} id="bottom" />
      <Handle type="source" position={Position.Right} id="right-s" />
      <Handle type="source" position={Position.Left} id="left-s" />
      <div className="fc-process-label">{d.label}</div>
      {d.agent && <div className="fc-process-agent">{d.agent}</div>}
      {d.optional && <div className="fc-optional-tag">optional</div>}
    </div>
  );
}

function DecisionNode({ data, selected }: NodeProps) {
  const d = data as { label: string };
  return (
    <div className={`fc-decision-wrap ${selected ? 'selected' : ''}`}>
      <Handle type="target" position={Position.Top} id="top" />
      <Handle type="target" position={Position.Right} id="right-t" />
      <Handle type="target" position={Position.Left} id="left-t" />
      <Handle type="source" position={Position.Bottom} id="bottom" />
      <Handle type="source" position={Position.Right} id="right-s" />
      <Handle type="source" position={Position.Left} id="left-s" />
      <div className="fc-decision">
        <span className="fc-decision-label">{d.label}</span>
      </div>
    </div>
  );
}

const nodeTypes: NodeTypes = {
  terminal: TerminalNode,
  process: ProcessNode,
  decision: DecisionNode,
};

// ---- Sizes for layout ----
const SIZES: Record<string, { w: number; h: number }> = {
  terminal: { w: 110, h: 44 },
  process: { w: 180, h: 60 },
  decision: { w: 130, h: 100 },
};

function rfType(t: WFNode['type']): string {
  if (t === 'start' || t === 'end') return 'terminal';
  return t;
}

function buildFlowchart(workflow: Workflow) {
  // Build React Flow nodes
  const rfNodes: Node[] = workflow.nodes.map(n => ({
    id: n.id,
    type: rfType(n.type),
    position: { x: 0, y: 0 },
    data: {
      label: n.label,
      agent: n.agent,
      optional: n.optional,
      description: n.description,
      wfType: n.type,
    },
  }));

  // Track loop edges to assign nesting offsets
  let loopIdx = 0;

  const rfEdges: Edge[] = workflow.edges.map((e, i) => {
    const isLoop = !!e.loop;
    const fromDecision = workflow.nodes.find(n => n.id === e.from)?.type === 'decision';

    let stroke = '#58a6ff';
    if (isLoop) stroke = '#f85149';
    else if (fromDecision) stroke = '#3fb950';

    const pathOptions = isLoop
      ? { offset: 25 + loopIdx * 45, borderRadius: 12 }
      : { borderRadius: 8 };

    const edge = {
      id: `${e.from}-${e.to}-${i}`,
      source: e.from,
      target: e.to,
      type: 'smoothstep',
      label: e.label,
      labelStyle: { fill: stroke, fontSize: 11, fontFamily: 'var(--font-mono)', fontWeight: 600 },
      labelBgStyle: { fill: '#0d1117', fillOpacity: 0.9 },
      labelBgPadding: [4, 6] as [number, number],
      labelBgBorderRadius: 3,
      style: { stroke, strokeWidth: 1.75, strokeDasharray: isLoop ? '6 4' : undefined },
      markerEnd: { type: MarkerType.ArrowClosed, color: stroke, width: 16, height: 16 },
      sourceHandle: isLoop ? 'right-s' : 'bottom',
      targetHandle: isLoop ? 'right-t' : 'top',
      pathOptions,
    } as Edge;

    if (isLoop) loopIdx++;

    return edge;
  });

  // Layout with dagre using only forward (non-loop) edges to avoid cycles
  const g = new dagre.graphlib.Graph();
  g.setDefaultEdgeLabel(() => ({}));
  g.setGraph({ rankdir: 'TB', nodesep: 70, ranksep: 65, marginx: 80, marginy: 40 });

  workflow.nodes.forEach(n => {
    const size = SIZES[rfType(n.type)];
    g.setNode(n.id, { width: size.w, height: size.h });
  });

  workflow.edges.forEach(e => {
    if (!e.loop) g.setEdge(e.from, e.to);
  });

  dagre.layout(g);

  const layouted = rfNodes.map(node => {
    const pos = g.node(node.id);
    const size = SIZES[node.type as string];
    return {
      ...node,
      position: { x: pos.x - size.w / 2, y: pos.y - size.h / 2 },
    };
  });

  return { nodes: layouted, edges: rfEdges };
}

function FlowchartInner({ workflow }: { workflow: Workflow }) {
  const navigate = useNavigate();
  const { nodes: flowNodes, edges: flowEdges } = useMemo(
    () => buildFlowchart(workflow),
    [workflow]
  );

  const [nodes, setNodes, onNodesChange] = useNodesState(flowNodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(flowEdges);
  const [selectedNode, setSelectedNode] = useState<WFNode | null>(null);

  useEffect(() => {
    setNodes(flowNodes);
    setEdges(flowEdges);
    setSelectedNode(null);
  }, [flowNodes, flowEdges, setNodes, setEdges]);

  const onNodeClick: NodeMouseHandler = useCallback((_, node) => {
    const wfNode = workflow.nodes.find(n => n.id === node.id);
    if (wfNode && (wfNode.type === 'process' || wfNode.type === 'decision')) {
      setSelectedNode(prev => prev?.id === wfNode.id ? null : wfNode);
    }
  }, [workflow]);

  return (
    <div className="flowchart-container">
      <div className="flowchart-canvas">
        <ReactFlow
          nodes={nodes}
          edges={edges}
          nodeTypes={nodeTypes}
          onNodesChange={onNodesChange}
          onEdgesChange={onEdgesChange}
          onNodeClick={onNodeClick}
          fitView
          fitViewOptions={{ padding: 0.25 }}
          minZoom={0.3}
          maxZoom={2}
          proOptions={{ hideAttribution: true }}
          nodesDraggable={false}
          nodesConnectable={false}
        >
          <Background color="#21262d" gap={20} />
          <Controls
            showInteractive={false}
            style={{ background: '#161b22', border: '1px solid #30363d', borderRadius: '6px' }}
          />
        </ReactFlow>
      </div>

      {selectedNode && (
        <div className="flowchart-detail">
          <div className="flowchart-detail-header">
            <span className={`detail-kind kind-${selectedNode.type}`}>{selectedNode.type}</span>
            <strong>{selectedNode.label}</strong>
            {selectedNode.agent && <span className="detail-agent">{selectedNode.agent}</span>}
            {selectedNode.optional && <span className="detail-optional-badge">optional</span>}
            {selectedNode.agent && (
              <button
                className="detail-view-agent-btn"
                onClick={() => navigate(`/catalog?node=agent:${encodeURIComponent(selectedNode.agent!)}`)}
              >
                View agent →
              </button>
            )}
          </div>
          {selectedNode.description && <p className="detail-desc">{selectedNode.description}</p>}
        </div>
      )}
    </div>
  );
}

export function WorkflowsView() {
  const [selectedWorkflow, setSelectedWorkflow] = useState<Workflow>(data.workflows[0]);

  return (
    <ReactFlowProvider>
      <div className="workflows-view">
        <div className="workflows-sidebar">
          <div className="sidebar-header">
            <h3 className="sidebar-title">Workflows</h3>
          </div>
          <div className="sidebar-list">
            {data.workflows.map(wf => (
              <button
                key={wf.id}
                className={`sidebar-item ${selectedWorkflow.id === wf.id ? 'active' : ''}`}
                onClick={() => setSelectedWorkflow(wf)}
              >
                <span className="sidebar-item-name">{wf.name}</span>
                <span className="sidebar-item-steps">{wf.nodes.filter(n => n.type === 'process').length} steps</span>
              </button>
            ))}
          </div>
          <div className="sidebar-legend">
            <div className="legend-row"><span className="legend-swatch swatch-process" />Process</div>
            <div className="legend-row"><span className="legend-swatch swatch-decision" />Decision</div>
            <div className="legend-row"><span className="legend-line line-pass" />Pass / yes</div>
            <div className="legend-row"><span className="legend-line line-loop" />Loop back</div>
          </div>
        </div>

        <div className="workflows-main">
          <div className="workflow-header">
            <h2 className="workflow-title">{selectedWorkflow.name}</h2>
            <p className="workflow-desc">{selectedWorkflow.description}</p>
          </div>
          <FlowchartInner key={selectedWorkflow.id} workflow={selectedWorkflow} />
        </div>
      </div>
    </ReactFlowProvider>
  );
}
