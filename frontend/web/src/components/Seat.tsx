import type { Player } from '../types';
import { Card } from './Card';

interface SeatProps {
  player: Player | null;
  isHero: boolean;
  className?: string;
}

export function Seat({ player, isHero, className = '' }: SeatProps) {
  if (!player) {
    return null;
  }

  return (
    <div className={`plaque-wrap ${className}`}>
      <div className="plaque-cards">
        {(player.holeCards ?? [undefined, undefined]).slice(0, 2).map((card, index) => (
          <Card key={`${player.id}-hole-${index}`} card={card} hidden={!isHero} compact />
        ))}
      </div>
      <div className={`plaque ${isHero ? 'plaque-hero' : ''} ${player.isTurn ? 'plaque-turn' : ''}`}>
        <div className="plaque-name">{isHero ? 'USER' : 'AGENT'}: {player.name.toUpperCase()}</div>
        <div className="plaque-stack">STACK {player.stack} {player.bet > 0 ? `// BET ${player.bet}` : ''}</div>
      </div>
    </div>
  );
}
