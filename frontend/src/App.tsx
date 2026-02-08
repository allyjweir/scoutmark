import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { ThemeProvider, BaseStyles, Spinner, Box } from '@primer/react';
import { AuthProvider, useAuth } from './hooks/useAuth';
import { useWebSocket } from './hooks/useWebSocket';
import { LoginPage } from './pages/LoginPage';
import { DashboardPage } from './pages/DashboardPage';
import { ScoringPage } from './pages/ScoringPage';
import type { ReactNode } from 'react';

const ProtectedRoute = ({ children }: { children: ReactNode }) => {
  const { user, loading } = useAuth();

  if (loading) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center" minHeight="100vh">
        <Spinner size="large" />
      </Box>
    );
  }

  if (!user) {
    return <Navigate to="/login" replace />;
  }

  return <>{children}</>;
};

const WebSocketProvider = ({ children }: { children: ReactNode }) => {
  const { user } = useAuth();
  // Only connect WebSocket when logged in
  useWebSocket();
  return user ? <>{children}</> : <>{children}</>;
};

const AppRoutes = () => (
  <WebSocketProvider>
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route
        path="/"
        element={
          <ProtectedRoute>
            <DashboardPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/sessions/:sessionId"
        element={
          <ProtectedRoute>
            <ScoringPage />
          </ProtectedRoute>
        }
      />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  </WebSocketProvider>
);

export const App = () => (
  <ThemeProvider colorMode="auto">
    <BaseStyles>
      <BrowserRouter>
        <AuthProvider>
          <AppRoutes />
        </AuthProvider>
      </BrowserRouter>
    </BaseStyles>
  </ThemeProvider>
);
