import { useMemo } from 'react';
import { getQuickBetAmount } from '../lib/pokerLogic';

interface ActionControlsProps {
  isTurn: boolean;
  stack: number;
  toCall: number;
  minRaise: number;
  pot: number;
  raiseAmount: number;
  onRaiseAmountChange: (next: number) => void;
  onAction: (type: 'fold' | 'check' | 'call' | 'bet' | 'raise', amount?: number) => void;
}

export function ActionControls({
  isTurn,
  stack,
  toCall,
  minRaise,
  pot,
  raiseAmount,
  onRaiseAmountChange,
  onAction,
}: ActionControlsProps) {
  const canCheck = toCall === 0;
  const safeMin = Math.min(stack, minRaise);
  const safeRaise = useMemo(() => Math.max(0, Math.floor(raiseAmount || 0)), [raiseAmount]);

  const handlePreset = (preset: 'min' | 'half-pot' | 'pot' | 'max') => {
    onRaiseAmountChange(getQuickBetAmount({ preset, minRaise: safeMin, toCall, pot, stack }));
  };

  const handleManualRaiseChange = (rawValue: string) => {
    if (rawValue.trim() === '') {
      onRaiseAmountChange(0);
      return;
    }

    const parsed = Number(rawValue);
    if (Number.isFinite(parsed)) {
      onRaiseAmountChange(Math.floor(parsed));
    }
  };

  const raiseIsValid = safeRaise >= safeMin && safeRaise <= stack;

  if (!isTurn) {
    return <div className="action-panel waiting">AWAITING_OPPONENT_ACTION...</div>;
  }

  return (
    <div className="action-panel">
      <div className="action-stack">
        <button type="button" className="enter-btn" onClick={() => onAction('fold')}>
          FOLD
        </button>

        {canCheck ? (
          <button type="button" className="enter-btn" onClick={() => onAction('check')}>
            CHECK
          </button>
        ) : (
          <button type="button" className="enter-btn" onClick={() => onAction('call')}>
            CALL_{toCall}
          </button>
        )}
      </div>

      <div className="raise-controls">
        <div className="raise-input-row">
          <label htmlFor="raise-input">RAISE TO</label>
          <input
            id="raise-input"
            type="number"
            step={1}
            value={safeRaise}
            onChange={(event) => handleManualRaiseChange(event.target.value)}
          />
        </div>

        <div className="preset-buttons">
          <button type="button" onClick={() => handlePreset('min')}>MIN</button>
          <button type="button" onClick={() => handlePreset('half-pot')}>1/2POT</button>
          <button type="button" onClick={() => handlePreset('pot')}>POT</button>
          <button type="button" onClick={() => handlePreset('max')}>MAX</button>
        </div>

        <button
          type="button"
          className="enter-btn full"
          disabled={!raiseIsValid}
          onClick={() => onAction(toCall === 0 ? 'bet' : 'raise', safeRaise)}
        >
          {toCall === 0 ? 'BET_TO' : 'RAISE_TO'}_{safeRaise}
        </button>
      </div>
    </div>
  );
}
