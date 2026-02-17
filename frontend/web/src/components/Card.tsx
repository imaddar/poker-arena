import type { Card as CardType } from '../types';

interface CardProps {
  card?: CardType;
  hidden?: boolean;
  compact?: boolean;
}

const SUITS: Record<CardType['suit'], string> = {
  s: '♠',
  h: '♥',
  d: '♦',
  c: '♣',
};

const RED_SUITS: CardType['suit'][] = ['h', 'd'];

export function Card({ card, hidden, compact }: CardProps) {
  if (hidden || !card) {
    return (
      <div className={`poker-card ${compact ? 'compact' : ''} card-back`} aria-label="Face down card">
        <div className="card-back-inner">
          <span className="card-back-emblem">PA</span>
        </div>
      </div>
    );
  }

  const colorClass = RED_SUITS.includes(card.suit) ? 'red' : 'black';

  return (
    <div className={`poker-card flip-in ${compact ? 'compact' : ''}`}>
      <span className={`card-value ${colorClass}`}>{card.rank}{SUITS[card.suit]}</span>
    </div>
  );
}
