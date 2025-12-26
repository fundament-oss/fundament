import { useEffect, useState } from 'react';
import {
  getLoginUrl,
  getUserInfo,
  logout,
  refreshToken,
  type UserInfo,
} from './api';
import './App.css';

type View = 'login' | 'loading' | 'user';

function App() {
  const [view, setView] = useState<View>('loading');
  const [user, setUser] = useState<UserInfo | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);

  useEffect(() => {
    checkAuth();
  }, []);

  async function checkAuth() {
    try {
      const userInfo = await getUserInfo();
      setUser(userInfo);
      setView('user');
      setError(null);
    } catch {
      // Not authenticated, show login
      setView('login');
    }
  }

  function initiateLogin() {
    // Redirect to API's login endpoint which handles the OIDC flow
    window.location.href = getLoginUrl();
  }

  async function handleRefreshToken() {
    try {
      await refreshToken();
      setMessage('Token refreshed successfully!');
      setTimeout(() => setMessage(null), 3000);
    } catch (err) {
      setError(`Failed to refresh token: ${err instanceof Error ? err.message : 'Unknown error'}`);
    }
  }

  async function handleLogout() {
    try {
      await logout();
      setUser(null);
      setView('login');
    } catch (err) {
      setError(`Failed to logout: ${err instanceof Error ? err.message : 'Unknown error'}`);
    }
  }

  return (
    <div className="container">
      {view === 'login' && (
        <div className="card">
          <h1>Fundament</h1>
          <p className="subtitle">Sign in to continue</p>
          <button className="btn btn-primary" onClick={initiateLogin}>
            Sign in with OIDC
          </button>
          {error && <div className="error">{error}</div>}
        </div>
      )}

      {view === 'loading' && (
        <div className="card">
          <div className="spinner" />
          <p>Loading...</p>
        </div>
      )}

      {view === 'user' && user && (
        <div className="card">
          <h1>Welcome</h1>
          <div className="user-info">
            <div className="info-row">
              <span className="label">Name:</span>
              <span className="value">{user.name || 'N/A'}</span>
            </div>
            <div className="info-row">
              <span className="label">User ID:</span>
              <span className="value">{user.id || 'N/A'}</span>
            </div>
            <div className="info-row">
              <span className="label">Tenant ID:</span>
              <span className="value">{user.tenantId || 'N/A'}</span>
            </div>
            <div className="info-row">
              <span className="label">Groups:</span>
              <span className="value">
                {user.groups && user.groups.length > 0 ? user.groups.join(', ') : 'None'}
              </span>
            </div>
          </div>
          <div className="button-group">
            <button className="btn btn-secondary" onClick={handleRefreshToken}>
              Refresh Token
            </button>
            <button className="btn btn-danger" onClick={handleLogout}>
              Sign Out
            </button>
          </div>
          {message && <div className="success">{message}</div>}
          {error && <div className="error">{error}</div>}
        </div>
      )}
    </div>
  );
}

export default App;
