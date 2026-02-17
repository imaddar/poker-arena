import { CircleUserRound, LayoutGrid, LogOut, PlayCircle } from 'lucide-react';
import type { ReactNode } from 'react';
import { Link, Outlet, useLocation, useNavigate } from 'react-router-dom';
import { useAuth } from '../contexts/AuthContext';

function NavItem({ to, label, icon }: { to: string; label: string; icon: ReactNode }) {
  const location = useLocation();
  const active = location.pathname === to || (to !== '/' && location.pathname.startsWith(to));

  return (
    <Link
      to={to}
      className={`app-nav__item ${active ? 'app-nav__item--active' : ''}`}
      aria-current={active ? 'page' : undefined}
    >
      {icon}
      <span>{label}</span>
    </Link>
  );
}

export function Layout() {
  const { user, logout } = useAuth();
  const navigate = useNavigate();

  const handleLogout = () => {
    logout();
    navigate('/');
  };

  return (
    <div className="app-shell">
      <div className="ambient ambient--one" />
      <div className="ambient ambient--two" />

      <header className="topbar">
        <Link to="/" className="brand" aria-label="Poker Arena home">
          <span className="brand__mark">PA</span>
          <span className="brand__text">Poker Arena</span>
        </Link>

        <nav className="app-nav" aria-label="Primary">
          <NavItem to="/" label="Play" icon={<PlayCircle size={16} />} />
          <NavItem to="/lobby" label="Lobby" icon={<LayoutGrid size={16} />} />
        </nav>

        <div className="topbar__right">
          <div className="user-chip">
            <CircleUserRound size={16} />
            <span>{user?.name ?? 'Guest'}</span>
          </div>
          {user && (
            <button type="button" className="ghost-btn" onClick={handleLogout}>
              <LogOut size={14} />
              <span>Sign out</span>
            </button>
          )}
        </div>
      </header>

      <main className="page-wrap">
        <Outlet />
      </main>
    </div>
  );
}
