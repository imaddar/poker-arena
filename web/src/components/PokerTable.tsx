import { ActionControls } from './ActionControls';
import { Card } from './Card';
import { Seat } from './Seat';
import type { ActionType, GameState, Player } from '../types';

interface PokerTableProps {
  gameState: GameState;
  tableInfo: string;
  raiseAmount: number;
  onRaiseAmountChange: (amount: number) => void;
  onAction: (type: ActionType, amount?: number) => void;
}

const POSITIONS: Record<number, string> = {
  0: 'seat-pos seat-pos--0',
  1: 'seat-pos seat-pos--1',
  2: 'seat-pos seat-pos--2',
  3: 'seat-pos seat-pos--3',
  4: 'seat-pos seat-pos--4',
  5: 'seat-pos seat-pos--5',
};

function findHeroSeat(seats: (Player | null)[]): number {
  const exact = seats.findIndex((seat) => seat?.id === 'hero');
  if (exact >= 0) {
    return exact;
  }

  return seats.findIndex((seat) => seat?.name.toLowerCase() === 'you');
}

function rotateSeat(index: number, heroSeat: number): number {
  if (heroSeat < 0) {
    return index;
  }

  return (index - heroSeat + 6) % 6;
}

export function PokerTable({ gameState, tableInfo, raiseAmount, onRaiseAmountChange, onAction }: PokerTableProps) {
  const heroSeat = findHeroSeat(gameState.seats);
  const hero = heroSeat >= 0 ? gameState.seats[heroSeat] : null;

  return (
    <section className="poker-arena">
      <div className="technical-table">
        <div className="felt-grid" />
        <div className="table-info-label">{tableInfo}</div>

        <div className="community">
          {gameState.communityCards.map((card, index) => (
            <Card key={`board-${index}`} card={card} />
          ))}
          {Array.from({ length: 5 - gameState.communityCards.length }).map((_, index) => (
            <Card key={`empty-${index}`} hidden />
          ))}
        </div>

        <div className="pot-label">POT_TOTAL: {gameState.pot}</div>

        {gameState.seats.map((player, index) => {
          const visualIndex = rotateSeat(index, heroSeat);
          const isHero = index === heroSeat;

          return (
            <Seat
              key={`seat-${index}`}
              player={player}
              isHero={isHero}
              className={POSITIONS[visualIndex]}
            />
          );
        })}
      </div>

      <ActionControls
        isTurn={Boolean(hero?.isTurn && hero.status === 'active')}
        stack={hero?.stack ?? 0}
        toCall={gameState.toCall}
        minRaise={gameState.minRaise}
        pot={gameState.pot}
        raiseAmount={raiseAmount}
        onRaiseAmountChange={onRaiseAmountChange}
        onAction={onAction}
      />
    </section>
  );
}
