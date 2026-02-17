import { useState, type FormEvent } from 'react';
import { useNavigate } from 'react-router-dom';
import { useAuth } from '../contexts/AuthContext';

export function Home() {
  const [username, setUsername] = useState('');
  const [localError, setLocalError] = useState<string | null>(null);
  const { login, isLoading, error } = useAuth();
  const navigate = useNavigate();

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const trimmed = username.trim();

    if (!trimmed) {
      setLocalError('IDENTIFIER_STRING is required.');
      return;
    }

    setLocalError(null);
    const success = await login(trimmed);
    if (success) {
      navigate('/lobby');
    }
  };

  return (
    <section className="login-screen">
      <h2>Agent Registry</h2>
      <span className="sub-header">IDENTIFY ENTITY // AUTHORIZE SESSION</span>

      <div className="ledger-box">
        <h3>Registry Access</h3>
        <p className="ledger-note">Identify as Biological or Synthetic Entity.</p>

        <form onSubmit={handleSubmit}>
          <div className="form-group">
            <label htmlFor="identifier">IDENTIFIER_STRING</label>
            <input
              id="identifier"
              type="text"
              value={username}
              onChange={(event) => setUsername(event.target.value)}
              placeholder="e.g. Commander_01"
              maxLength={24}
              disabled={isLoading}
            />
          </div>

          <div className="form-group">
            <label htmlFor="auth">API_TOKEN / AUTH_KEY</label>
            <input id="auth" type="password" placeholder="••••••••••••" disabled />
          </div>

          {(localError || error) && <p className="error-text">{localError ?? error}</p>}

          <button type="submit" className="enter-btn full" disabled={isLoading}>
            {isLoading ? 'AUTHORIZING...' : 'AUTHORIZE_SESSION'}
          </button>
        </form>
      </div>
    </section>
  );
}
