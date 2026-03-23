import { useState, useEffect } from 'react';

const messages = [
  'Sketching wireframes...',
  'Painting pixels...',
  'Arranging components...',
  'Polishing interfaces...',
  'Stringing beads...',
  'Compiling creativity...',
  'Assembling the view...',
];

const beads = [
  { symbol: '●', className: 'bead-pink' },
  { symbol: '◉', className: 'bead-cyan' },
  { symbol: '●', className: 'bead-amber' },
  { symbol: '◉', className: 'bead-purple' },
  { symbol: '●', className: 'bead-green' },
  { symbol: '◉', className: 'bead-pink' },
  { symbol: '●', className: 'bead-cyan' },
  { symbol: '◉', className: 'bead-amber' },
  { symbol: '●', className: 'bead-purple' },
];

export function BuildingAnimation() {
  const [messageIndex, setMessageIndex] = useState(0);

  useEffect(() => {
    const interval = setInterval(() => {
      setMessageIndex(i => (i + 1) % messages.length);
    }, 2500);
    return () => clearInterval(interval);
  }, []);

  return (
    <div className="building-animation">
      <pre className="building-robot">
{"        ♥\n"}
{"       ╱│╲\n"}
{"    ┌──────────┐\n"}
{"    │  ◠    ◠  │\n"}
{"    │    ▽     │\n"}
{"    │  ╰────╯  │\n"}
{"    └─────┬────┘\n"}
{"     "}
{beads.map((b, i) => (
  <span
    key={i}
    className={`bead ${b.className} building-bead`}
    style={{ animationDelay: `${i * 0.15}s` }}
  >{b.symbol}</span>
))}
{"\n"}
{"    ┌─────┴────┐\n"}
{"    │  ░▓░▓░▓  │\n"}
{"    │  ▓░▓░▓░  │\n"}
{"    └──┬────┬──┘\n"}
{"       │    │\n"}
{"      ═╧═  ═╧═"}
      </pre>
      <div className="building-status">
        <span className="building-dots">
          <span className="building-dot" />
          <span className="building-dot" />
          <span className="building-dot" />
        </span>
        <p key={messageIndex} className="building-message">
          {messages[messageIndex]}
        </p>
      </div>
    </div>
  );
}
