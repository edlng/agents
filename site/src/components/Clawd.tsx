import type { CSSProperties } from 'react';
import { useCallback, useEffect, useRef, useState } from 'react';
import { ACTION_MS, pickAction, type ClawdAction } from './clawdActions';
import './Clawd.css';

type Rect = readonly [x: number, y: number, w: number, h: number];

const BODY: Rect = [2, 0, 9, 5];
const EYES: Rect[] = [[4, 1, 1, 2], [8, 1, 1, 2]];
const ARM_LEFT: Rect = [0, 2, 2, 2];
const ARM_RIGHT: Rect = [11, 2, 2, 2];
const LEGS_A: Rect[] = [[3, 5, 1, 2], [7, 5, 1, 2]];
const LEGS_B: Rect[] = [[5, 5, 1, 2], [9, 5, 1, 2]];

function Rects({ rects }: { rects: readonly Rect[] }) {
  return rects.map(([x, y, w, h]) => <rect key={`${x}-${y}`} x={x} y={y} width={w} height={h} />);
}

/* Props that start hidden behind the body are drawn before it; the rest are drawn after. */

function Sunglasses() {
  return (
    <g className="clawd-glasses" fill="#0b0a09">
      <rect x="3" y="1" width="3" height="2" />
      <rect x="7" y="1" width="3" height="2" />
      <rect x="6" y="1" width="1" height="0.6" />
      <rect x="2" y="1" width="1" height="0.6" />
      <rect x="10" y="1" width="1" height="0.6" />
      <g fill="#fff">
        <rect className="clawd-glint" x="3.3" y="1.2" width="0.5" height="1.3" />
        <rect className="clawd-glint" x="7.3" y="1.2" width="0.5" height="1.3" />
      </g>
    </g>
  );
}

function Laptop() {
  return (
    <g className="clawd-laptop">
      <g className="clawd-screen">
        <rect x="3" y="3" width="7" height="3" fill="#0f0d0b" stroke="var(--text-muted)" strokeWidth="0.3" />
        <rect className="clawd-code clawd-code-1" x="3.8" y="3.6" width="3" height="0.5" fill="var(--accent-green)" />
        <rect className="clawd-code clawd-code-2" x="4.4" y="4.3" width="4.4" height="0.5" fill="var(--accent-yellow)" />
        <rect className="clawd-code clawd-code-3" x="3.8" y="5" width="2.2" height="0.5" fill="var(--accent-teal)" />
      </g>
      <rect className="clawd-keys" x="2" y="6" width="9" height="0.8" fill="var(--text-muted)" />
    </g>
  );
}

function Mug() {
  return (
    <g className="clawd-mug">
      <rect x="12" y="0" width="2" height="2.5" fill="var(--text-primary)" />
      <rect x="12.3" y="0.3" width="1.4" height="0.5" fill="#6b3f2a" />
      <rect x="14" y="0.6" width="0.8" height="1.4" fill="none" stroke="var(--text-primary)" strokeWidth="0.4" />
    </g>
  );
}

function Steam() {
  return (
    <g className="clawd-steam" fill="var(--text-secondary)">
      <rect className="clawd-wisp clawd-wisp-1" x="12.4" y="-1.9" width="0.4" height="1.2" />
      <rect className="clawd-wisp clawd-wisp-2" x="13.2" y="-1.9" width="0.4" height="1.2" />
    </g>
  );
}

function Bulb() {
  return (
    <g className="clawd-bulb">
      <circle className="clawd-glow" cx="6.5" cy="-2.5" r="3.4" fill="var(--accent-yellow)" opacity="0.16" />
      <rect x="5" y="-4" width="3" height="3" fill="var(--accent-yellow)" />
      <rect x="5.5" y="-1" width="2" height="0.8" fill="var(--text-muted)" />
      <g className="clawd-rays" fill="var(--accent-yellow)">
        <rect x="3.4" y="-3.2" width="0.9" height="0.4" />
        <rect x="8.7" y="-3.2" width="0.9" height="0.4" />
        <rect x="6.3" y="-5.4" width="0.4" height="0.9" />
      </g>
    </g>
  );
}

function Sign() {
  return (
    <g className="clawd-sign">
      <rect x="6.25" y="-2" width="0.5" height="2.2" fill="var(--text-muted)" />
      <rect x="2.5" y="-6" width="8" height="4" fill="var(--accent-green)" />
      <text
        x="6.5"
        y="-3.1"
        textAnchor="middle"
        fontSize="2.1"
        fontWeight="700"
        fill="var(--bg-primary)"
        style={{ fontFamily: 'var(--font-mono)' }}
      >
        LGTM
      </text>
    </g>
  );
}

function Boombox() {
  return (
    <g className="clawd-boombox">
      <rect x="-4.8" y="4.2" width="4" height="2.8" fill="var(--text-muted)" />
      <rect x="-4.3" y="3.6" width="3" height="0.6" fill="var(--text-muted)" />
      <rect className="clawd-speaker" x="-4.4" y="4.7" width="1.4" height="1.4" fill="var(--bg-primary)" />
      <rect className="clawd-speaker" x="-2.6" y="4.7" width="1.4" height="1.4" fill="var(--bg-primary)" />
    </g>
  );
}

function Note({ className, x }: { className: string; x: number }) {
  return (
    <g transform={`translate(${x} 0)`}>
      <g className={`clawd-note ${className}`} fill="var(--accent-magenta)">
        <rect x="1" y="-2" width="0.5" height="2.2" />
        <rect x="0" y="1" width="1.5" height="1" />
        <rect x="1.5" y="-2" width="1" height="0.6" />
      </g>
    </g>
  );
}

function Zzz() {
  return (
    <g className="clawd-zzz" fill="none" stroke="var(--text-secondary)" strokeWidth="0.4">
      {[0, 1, 2].map(i => (
        <path key={i} className={`clawd-z clawd-z-${i}`} d="M11.5 0h1.6l-1.6 1.6h1.6" />
      ))}
    </g>
  );
}

interface ClawdProps {
  size?: number;
  mode?: 'idle' | 'walk';
  /** Plays a random action every few seconds. */
  roam?: boolean;
  /** Clicking plays a random action right away. */
  interactive?: boolean;
  onAction?: (action: ClawdAction | null) => void;
}

export function Clawd({ size = 48, mode = 'idle', roam = false, interactive = false, onAction }: ClawdProps) {
  const [action, setAction] = useState<ClawdAction | null>(null);
  const timer = useRef<number>(undefined);
  const current = useRef<ClawdAction | null>(null);
  const onActionRef = useRef(onAction);
  onActionRef.current = onAction;

  const play = useCallback((next: ClawdAction | null) => {
    window.clearTimeout(timer.current);
    current.current = next;
    setAction(next);
    onActionRef.current?.(next);
    if (next) {
      timer.current = window.setTimeout(() => play(null), ACTION_MS[next]);
    } else if (roam) {
      const gap = 1500 + Math.random() * 2500;
      timer.current = window.setTimeout(() => play(pickAction(null)), gap);
    }
  }, [roam]);

  useEffect(() => {
    if (!roam || window.matchMedia('(prefers-reduced-motion: reduce)').matches) return;
    timer.current = window.setTimeout(() => play(pickAction(null)), 800);
    return () => window.clearTimeout(timer.current);
  }, [roam, play]);

  const trigger = () => play(pickAction(current.current));

  const classes = ['clawd', `clawd-${mode}`, action ? `clawd-act-${action}` : ''].join(' ');

  const style = action ? ({ '--dur': `${ACTION_MS[action]}ms` } as CSSProperties) : undefined;

  return (
    <svg
      className={classes}
      style={style}
      width={size}
      height={(size * 7) / 13}
      viewBox="0 0 13 7"
      shapeRendering="geometricPrecision"
      role={interactive ? 'button' : 'img'}
      aria-label="Clawd the crab"
      tabIndex={interactive ? 0 : undefined}
      onClick={interactive ? trigger : undefined}
      onKeyDown={interactive ? e => (e.key === 'Enter' || e.key === ' ') && trigger() : undefined}
    >
      <g className="clawd-legs" fill="var(--accent)">
        <g className="clawd-legs-a"><Rects rects={LEGS_A} /></g>
        <g className="clawd-legs-b"><Rects rects={LEGS_B} /></g>
      </g>
      {action === 'coffee' && <Mug />}
      {action === 'idea' && <Bulb />}
      {action === 'lgtm' && <Sign />}
      {action === 'dance' && <Boombox />}
      <g className="clawd-bob">
        <g className="clawd-body" fill="var(--accent)">
          <Rects rects={[BODY]} />
          <g className="clawd-arm clawd-arm-left"><Rects rects={[ARM_LEFT]} /></g>
          <g className="clawd-arm clawd-arm-right"><Rects rects={[ARM_RIGHT]} /></g>
          <g className="clawd-look">
            <g className="clawd-eyes" fill="var(--bg-primary)"><Rects rects={EYES} /></g>
          </g>
          {action === 'sunglasses' && <Sunglasses />}
        </g>
      </g>
      {action === 'laptop' && <Laptop />}
      {action === 'coffee' && <Steam />}
      {action === 'dance' && (
        <g className="clawd-notes">
          <Note className="clawd-note-1" x={-1} />
          <Note className="clawd-note-2" x={11} />
        </g>
      )}
      {action === 'sleep' && <Zzz />}
    </svg>
  );
}

/** A crab that patrols back and forth along a track, with a different pace and start each visit. */
export function ClawdWalker() {
  const [pace] = useState(() => {
    const duration = 30 + Math.random() * 25;
    return { duration, delay: -Math.random() * duration };
  });
  const style = { animationDuration: `${pace.duration}s`, animationDelay: `${pace.delay}s` };

  return (
    <div className="clawd-track" aria-hidden="true">
      <div className="clawd-walker" style={style}>
        <div className="clawd-flipper" style={style}>
          <Clawd size={28} mode="walk" />
        </div>
      </div>
    </div>
  );
}
